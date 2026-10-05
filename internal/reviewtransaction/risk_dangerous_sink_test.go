package reviewtransaction

import (
	"context"
	"testing"
)

func TestDangerousSinkLine(t *testing.T) {
	cases := []struct {
		name, path, line string
		want             bool
	}{
		{"TLS Go", "src/client.go", `tls.Config{InsecureSkipVerify: true}`, true},
		{"TLS Rust", "src/client.rs", `builder.danger_accept_invalid_certs(true);`, true},
		{"TLS safe", "src/client.py", `requests.get(url, verify=True)`, false},
		{"Java empty unrelated verify method", "src/Policy.java", `void verify() {}`, false},
		{"Java trust all hostname", "src/TLS.java", `HostnameVerifier verifier = (hostname, session) -> true;`, true},
		{"deserialization Python", "src/load.py", `value = pickle.loads(payload)`, true},
		{"deserialization Java", "src/Load.java", `new ObjectInputStream(stream)`, true},
		{"deserialization safe", "src/load.py", `yaml.load(payload, Loader=yaml.SafeLoader)`, false},
		{"safe loader beside unsafe load", "src/load.py", `safe = yaml.load(a, Loader=yaml.SafeLoader); unsafe = yaml.load(payload)`, true},
		{"unrelated SafeLoader beside unsafe load", "src/load.py", `loader = yaml.SafeLoader; unsafe = yaml.load(payload)`, true},
		{"safe load", "src/load.py", `yaml.safe_load(payload)`, false},
		{"evaluation JS", "src/run.ts", `result = eval(input)`, true},
		{"evaluation PHP", "src/run.php", `eval($expression);`, true},
		{"PHP boolean assertion", "src/run.php", `assert($valid);`, false},
		{"evaluation safe", "src/run.ts", `result = evaluate(input)`, false},
		{"evaluation member", "src/run.ts", `result = object.eval(input)`, false},
		{"shell Python", "src/run.py", `subprocess.run(command, shell=True)`, true},
		{"shell Ruby", "src/run.rb", "result = `echo #{name}`", true},
		{"shell literal", "src/run.py", `subprocess.run(["sh", "-c", "ls"])`, false},
		{"literal command with shell", "src/run.py", `subprocess.run("ls", shell=True)`, false},
		{"literal-only system concatenation", "src/run.py", `os.system("echo" + " hi")`, false},
		{"unrelated Python shell flag", "src/run.py", `options = dict(shell=True)`, false},
		{"Python system arithmetic argument", "src/run.py", `os.system("exit " + str(code))`, true},
		{"weak hash Python", "src/auth.py", `digest = hashlib.md5(password).hexdigest()`, true},
		{"weak hash JS", "src/auth.ts", `const hashedPassword = crypto.createHash('sha1').update(password).digest('hex')`, true},
		{"weak hash checksum", "src/hash.py", `digest = hashlib.md5(file_bytes).hexdigest()`, false},
		{"weak hash unrelated token variable", "src/hash.py", `token = hashlib.md5(file_bytes).hexdigest()`, false},
		{"weak hash nearby secret log", "src/hash.py", `digest = hashlib.md5(file_bytes).hexdigest(); log(secret)`, false},
		{"weak hash Go credential", "src/auth.go", `digest := sha1.Sum(password)`, true},
		{"weak hash PHP credential", "src/auth.php", `$digest = md5($secret);`, true},
		{"CORS JS", "src/server.js", `res.setHeader('Access-Control-Allow-Origin', '*'); res.setHeader('Access-Control-Allow-Credentials', 'true')`, true},
		{"CORS PHP", "src/server.php", `header('Access-Control-Allow-Origin: *'); header('Access-Control-Allow-Credentials: true');`, true},
		{"CORS without credentials", "src/server.js", `res.setHeader('Access-Control-Allow-Origin', '*')`, false},
		{"permissions Go", "src/files.go", `os.Chmod(name, 0o777)`, true},
		{"permissions shell", "src/setup.sh", `chmod -R 777 /data`, true},
		{"permissions safe", "src/files.go", `os.Chmod(name, 0755)`, false},
		{"JWT Python", "src/auth.py", `jwt.decode(token, options={"verify_signature": False})`, true},
		{"JWT Go", "src/auth.go", `parser.ParseUnverified(token, claims)`, true},
		{"JWT JS decode", "src/auth.js", `jwt.decode(token)`, false},
		{"CSRF Python", "src/views.py", `@csrf_exempt`, true},
		{"CSRF Java", "src/Web.java", `http.csrf().disable();`, true},
		{"CSRF safe", "src/views.py", `WTF_CSRF_ENABLED = True`, false},
		{"XXE Python", "src/xml.py", `etree.XMLParser(resolve_entities=True)`, true},
		{"XXE C sharp", "src/Xml.cs", `reader.DtdProcessing = DtdProcessing.Parse;`, true},
		{"XXE safe", "src/xml.py", `etree.XMLParser(resolve_entities=False)`, false},
		{"comment", "src/run.ts", `  // eval(input)`, false},
		{"test path", "src/run_test.go", `tls.Config{InsecureSkipVerify: true}`, false},
		{"python test path", "src/test_auth.py", `jwt.decode(token, verify=False)`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dangerousSinkLine(tc.path, tc.line); got != tc.want {
				t.Fatalf("dangerousSinkLine(%q, %q) = %v, want %v", tc.path, tc.line, got, tc.want)
			}
		})
	}
}

func TestAssessSnapshotRiskDangerousSinkAddedLines(t *testing.T) {
	for _, tc := range []struct {
		name, path, before, after string
		high                      bool
	}{
		{"added TLS bypass", "src/client.go", "package client\nvar x = 1\n", "package client\nvar x = 1\nvar config = tls.Config{InsecureSkipVerify: true}\n", true},
		{"added unsafe deserialization", "src/load.py", "value = 1\n", "value = 1\nvalue = pickle.loads(payload)\n", true},
		{"unsafe YAML after safe YAML", "src/load.py", "value = 1\n", "value = 1\nsafe = yaml.load(a, Loader=yaml.SafeLoader); unsafe = yaml.load(payload)\n", true},
		{"safe YAML stays medium", "src/load.py", "value = 1\n", "value = 1\nsafe = yaml.load(payload, Loader=yaml.SafeLoader)\n", false},
		{"checksum assigned to token stays medium", "src/hash.py", "value = 1\n", "value = 1\ntoken = hashlib.md5(file_bytes).hexdigest()\n", false},
		{"unchanged sink", "src/client.go", "package client\nvar config = tls.Config{InsecureSkipVerify: true}\nvar x = 1\n", "package client\nvar config = tls.Config{InsecureSkipVerify: true}\nvar x = 2\n", false},
		{"removed sink", "src/client.go", "package client\nvar config = tls.Config{InsecureSkipVerify: true}\n", "package client\n", false},
		{"test sink", "src/client_test.go", "package client\n", "package client\nvar config = tls.Config{InsecureSkipVerify: true}\n", false},
		{"fixture sink", "fixtures/client.go", "package client\n", "package client\nvar config = tls.Config{InsecureSkipVerify: true}\n", false},
		{"process before sink", "src/client.go", "package client\n", "package client\nfunc run() { exec.Command(\"git\") }\nvar config = tls.Config{InsecureSkipVerify: true}\n", true},
		{"benign", "src/client.go", "package client\nvar x = 1\n", "package client\nvar x = 2\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, tc.path, tc.before)
			gitSnapshot(t, repo, "add", "--", tc.path)
			gitSnapshot(t, repo, "commit", "-m", "base")
			writeSnapshotFile(t, repo, tc.path, tc.after)
			snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
			if err != nil {
				t.Fatal(err)
			}
			assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			want := RiskMedium
			if tc.high {
				want = RiskHigh
			}
			if assessment.Level != want {
				t.Fatalf("assessment = %#v, want %s", assessment, want)
			}
			found := false
			for _, reason := range assessment.Reasons {
				if reason.Code == RiskReasonDangerousSink && reason.Path == tc.path {
					found = true
				}
			}
			if found != tc.high {
				t.Fatalf("assessment reasons = %#v, want dangerous_sink=%v", assessment.Reasons, tc.high)
			}
		})
	}
}
