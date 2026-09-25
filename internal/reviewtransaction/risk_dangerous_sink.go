package reviewtransaction

import (
	"path"
	"regexp"
	"strings"
)

// Each entry identifies a concrete sink, not a suspicious word. Expressions
// are compiled once; language gating prevents identical API names in unrelated
// runtimes from turning a deterministic high into a lexical false positive.
type dangerousSinkPattern struct {
	name       string
	extensions string
	expression *regexp.Regexp
}

func sink(name, extensions, expression string) dangerousSinkPattern {
	return dangerousSinkPattern{name, extensions, regexp.MustCompile(expression)}
}

var dangerousSinkCatalog = []dangerousSinkPattern{
	// CWE-295: disabled certificate validation.
	sink("TLS Go", ".go", `\bInsecureSkipVerify\s*:\s*true\b`),
	sink("TLS JS", ".js .jsx .ts .tsx", `\brejectUnauthorized\s*:\s*false\b|\bNODE_TLS_REJECT_UNAUTHORIZED\s*=\s*['"]?0\b`),
	sink("TLS Python", ".py", `\bverify\s*=\s*False\b|\bCERT_NONE\b|\bcheck_hostname\s*=\s*False\b`),
	sink("TLS Rust", ".rs", `\.danger_accept_invalid_certs\s*\(\s*true\s*\)`),
	sink("TLS Ruby", ".rb", `OpenSSL::SSL::VERIFY_NONE\b`),
	sink("TLS PHP", ".php", `\bCURLOPT_SSL_VERIFYPEER\s*=>\s*(?:false|0)\b`),
	sink("TLS C sharp", ".cs", `ServerCertificateValidationCallback\s*\+=?\s*(?:\([^)]*\)|\w+)\s*=>\s*true\b`),
	sink("TLS shell", ".sh .bash .zsh", `\bcurl\s+(?:[^;|]*\s)?(?:--insecure|-k)(?:\s|$)`),
	sink("TLS Java trust all", ".java .kt .kts", `\bcheckServerTrusted\s*\([^)]*\)\s*\{\s*\}|\bHostnameVerifier\b[^;\n]*\([^)]*\)\s*->\s*true\b`),
	// CWE-502: deserializing untrusted object graphs.
	sink("pickle", ".py", `\bpickle\.loads?\s*\(`),
	sink("unsafe YAML Python", ".py", `\byaml\.(?:unsafe_load|load)\s*\(`),
	sink("marshal Python", ".py", `\bmarshal\.loads?\s*\(`),
	sink("Java object stream", ".java .kt .kts", `\bObjectInputStream\s*\(`),
	sink("C sharp formatter", ".cs", `\bBinaryFormatter\s*\(`),
	sink("PHP unserialize", ".php", `(?:^|[^\w.])unserialize\s*\(`),
	sink("Ruby marshal", ".rb", `\bMarshal\.load\s*\(`),
	sink("Ruby YAML", ".rb", `\bYAML\.load\s*\(`),
	sink("C sharp type names", ".cs", `\bTypeNameHandling\s*(?:=|:)\s*TypeNameHandling\.(?:All|Auto)\b`),
	sink("node serialize", ".js .jsx .ts .tsx", `\b(?:serialize|nodeSerialize)\.unserialize\s*\(`),
	// CWE-94: runtime evaluation of supplied code.
	sink("eval", ".js .jsx .ts .tsx .py .php .rb", `(?:^|[^\w.])eval\s*\(`),
	sink("Function constructor", ".js .jsx .ts .tsx", `\bnew\s+Function\s*\(`),
	sink("node VM", ".js .jsx .ts .tsx", `\bvm\.runInNewContext\s*\(`),
	sink("PHP create_function", ".php", `\bcreate_function\s*\(`),
	sink("Ruby string eval", ".rb", "\\b(?:instance_eval|class_eval)\\s*\\(\\s*[\"'`]"),
	// CWE-78: shell interpretation of composed commands.
	sink("Python shell", ".py", `\bsubprocess\.(?:run|Popen|call|check_call|check_output)\s*\(\s*(?:[A-Za-z_]\w*|f["']|["'][^"']*["']\s*\+)[^;\n]*\bshell\s*=\s*True\b`),
	sink("Python system composition", ".py", `\bos\.system\s*\(\s*(?:f["']|[^)\n]*(?:\+\s*[A-Za-z_]\w*|[A-Za-z_]\w*\s*\+|%\s*[A-Za-z_]\w*|\.format\s*\(\s*[A-Za-z_]\w*))`),
	sink("PHP shell variable", ".php", "\\b(?:system|exec|shell_exec|passthru)\\s*\\(\\s*\\$\\w+|`[^`]*\\$\\w+[^`]*`"),
	sink("Ruby interpolated shell", ".rb", "`[^`]*#\\{[^}]+\\}[^`]*`|%x[({][^)}]*#\\{[^}]+\\}"),
	sink("composed sh -c", ".go .java .kt .kts .js .jsx .ts .tsx .py .rb .rs .cs", "(?:[\"'](?:sh|bash)[\"']\\s*,\\s*[\"']-c[\"']\\s*,\\s*(?:\\w+|[\"'][^\"']*[\"']\\s*\\+|f[\"']|`))"),
	// CWE-327: weak digest specifically for a credential, not a file checksum.
	sink("weak credential hash", ".go .java .kt .kts .js .jsx .ts .tsx .py .php .rb .rs .cs", `(?i)\b(?:hashlib\.)?(?:md5|sha1)(?:\.Sum)?\s*\(\s*\$?\w*(?:password|passwd|secret|token)\w*\b|\bcreateHash\s*\(\s*['"](?:md5|sha1)['"]\s*\)\s*\.update\s*\(\s*\w*(?:password|passwd|secret|token)\w*\b|\bMessageDigest\.getInstance\s*\(\s*['"](?:MD5|SHA-1)['"]\s*\)\.digest\s*\(\s*\w*(?:password|passwd|secret|token)\w*\b`),
	// CWE-942: wildcard origin combined with credential sharing (same line).
	sink("credentialed wildcard CORS", ".go .java .kt .kts .js .jsx .ts .tsx .py .php .rb .rs .cs", `(?i)Access-Control-Allow-Origin[^;\n]*\*[^;\n]*;[^\n]*Access-Control-Allow-Credentials[^\n]*true`),
	// CWE-732: world-writable modes.
	sink("world writable API", ".go .js .jsx .ts .tsx .py .php .rb .rs .cs", `(?i)\b(?:chmod|Chmod|mkdir|WriteFile)\s*\([^\n]*\b(?:0o777|0777|0666)\b|\bumask\s*\(\s*0\s*\)`),
	sink("world writable shell", ".sh .bash .zsh", `\bchmod\s+(?:-R\s+)?(?:777\b|o\+w\b)`),
	// CWE-347: decoded tokens are not verified signatures.
	sink("JWT Python bypass", ".py", `\bjwt\.decode\s*\([^\n]*(?:\bverify\s*=\s*False\b|["']verify_signature["']\s*:\s*False\b|\balgorithms\s*=\s*\[\s*["']none["']\s*\])`),
	sink("JWT Go unverified", ".go", `\b(?:jwt|\w+)\.ParseUnverified\s*\(`),
	sink("JWT Java unsecured", ".java .kt .kts", `\.parseUnsecuredClaims\s*\(`),
	// CWE-352: explicit CSRF opt-outs.
	sink("CSRF Python", ".py", `@csrf_exempt\b|\bWTF_CSRF_ENABLED\s*=\s*False\b`),
	sink("CSRF Spring", ".java .kt .kts", `\.csrf\s*\(\s*\)\.disable\s*\(`),
	sink("CSRF Rails", ".rb", `\bskip_before_action\s+:verify_authenticity_token\b`),
	sink("CSRF config", ".js .jsx .ts .tsx .php .rb", `\bcsrf\s*:\s*false\b|\bcsrf\s*=>\s*false\b`),
	sink("CSRF C sharp", ".cs", `\[IgnoreAntiforgeryToken\]`),
	// CWE-611: external entities and DTD expansion.
	sink("XXE Python", ".py", `\bresolve_entities\s*=\s*True\b`),
	sink("XXE Java", ".java .kt .kts", `\bXMLInputFactory\.IS_SUPPORTING_EXTERNAL_ENTITIES\s*,\s*true\b|\.setExpandEntityReferences\s*\(\s*true\s*\)`),
	sink("XXE PHP", ".php", `\bLIBXML_NOENT\b`),
	sink("XXE C sharp", ".cs", `\bDtdProcessing\.Parse\b`),
}

func dangerousSinkLine(logicalPath, line string) bool {
	if isTestRiskPath(logicalPath) {
		return false
	}
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"//", "#", "--", "/*", "*", "<!--", "'''", `"""`} {
		if strings.HasPrefix(trimmed, prefix) {
			return false
		}
	}
	extension := strings.ToLower(path.Ext(logicalPath))
	for _, entry := range dangerousSinkCatalog {
		if !strings.Contains(" "+entry.extensions+" ", " "+extension+" ") || !entry.expression.MatchString(line) {
			continue
		}
		if entry.name == "unsafe YAML Python" {
			if strings.Contains(line, "yaml.unsafe_load(") || hasUnsafeYAMLLoad(line) {
				return true
			}
			continue
		}
		return true
	}
	return false
}

// A SafeLoader protects its own yaml.load call, not a different call or an
// unrelated assignment elsewhere on the same added line. Unbalanced calls
// remain unsafe evidence rather than silently discarding the signal.
var yamlLoadCall = regexp.MustCompile(`\byaml\.load\s*\(`)
var safeYAMLLoaderArg = regexp.MustCompile(`\bLoader\s*=\s*(?:yaml\.)?C?SafeLoader\b`)

func hasUnsafeYAMLLoad(line string) bool {
	for _, loc := range yamlLoadCall.FindAllStringIndex(line, -1) {
		start, depth, end := loc[1], 1, len(line)
		for i := start; i < len(line); i++ {
			switch line[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = i
					i = len(line)
				}
			}
		}
		if !safeYAMLLoaderArg.MatchString(line[start:end]) {
			return true
		}
	}
	return false
}
