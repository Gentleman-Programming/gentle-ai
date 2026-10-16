# Settings fixtures and repeated installed entries, bounded modified-Windows lab only.
$ErrorActionPreference = 'Stop'
$work = 'R:\Gentle Lab Work'
$utf8 = [Text.UTF8Encoding]::new($false)
function Read-Bounded([string]$path, [int]$bound) {
    if ((Get-Item -LiteralPath $path).Length -gt $bound) { throw 'Whole input withheld: bound exceeded' }
    return [IO.File]::ReadAllText($path, $utf8)
}
$control = Read-Bounded 'C:\ProgramData\gentle-lab35-control.json' 4096 | ConvertFrom-Json
$proof = Read-Bounded (Join-Path $work 'qualification-before-candidate.json') 4096 | ConvertFrom-Json
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
$rate = [GentleOSJob35]::VerifyCurrent() # Read-only live handle + physical current Job queries.
$volume = Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='R:'"
$os = Get-CimInstance Win32_OperatingSystem
$cpu = Get-CimInstance Win32_Processor
$credentials = @('GITHUB_TOKEN','GH_TOKEN','OPENAI_API_KEY','ANTHROPIC_API_KEY','AZURE_CLIENT_SECRET','AWS_SECRET_ACCESS_KEY')
foreach ($name in $credentials) { if ([Environment]::GetEnvironmentVariable($name)) { throw 'Credentialless Guest required' } }
if ($env:USERNAME -ne 'GentleLab35' -or $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) -or
    $identity.User.Value -ne $control.NormalSID -or $proof.SID -ne $identity.User.Value -or $proof.QualifiedProcessId -ne $PID -or
    -not $control.Nonce -or $proof.Nonce -cne $control.Nonce -or $proof.Elevated -ne $false -or $proof.CandidateExecuted -ne $false -or
    $proof.Result -ne 'QUALIFIED_BEFORE_CANDIDATE' -or @($proof.CredentialVariableNames).Count -ne 0 -or
    $proof.JobPhysicalReadback -ne $true -or $proof.FixedGuestVolumeReadback -ne $true -or $proof.JobMemoryBytes -ne 3221225472 -or
    $proof.JobActiveProcessLimit -ne 64 -or $proof.JobCPUFlags -ne 5 -or $proof.JobCPURate -ne $rate -or
    $rate -ne [math]::Floor(10000 / [Environment]::ProcessorCount) -or $proof.FileSystem -ne 'NTFS' -or
    $volume.FileSystem -ne 'NTFS' -or $volume.DriveType -ne 3 -or $volume.Size -lt 3000000000 -or $volume.Size -gt 3221225472 -or
    $proof.DiskCapacityBytes -ne $volume.Size -or $os.Caption -notlike '*Windows 11 Enterprise*' -or $os.BuildNumber -ne '26100' -or
    $os.ProductType -ne 1 -or @($cpu | Where-Object Architecture -ne 9).Count -ne 0 -or -not [Environment]::Is64BitProcess) {
    throw 'Fresh normal Win11 AMD64 physical qualification refused; no product read'
}
foreach ($path in @('R:\', $work)) {
    $acl = Get-Acl -LiteralPath $path
    if ($acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -ne $identity.User.Value -or -not $acl.AreAccessRulesProtected -or
        ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Private protected Guest root refused' }
    foreach ($rule in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) {
        if ($rule.AccessControlType -eq 'Allow' -and $rule.IdentityReference.Value -notin @($identity.User.Value,'S-1-5-18','S-1-5-32-544')) {
            throw 'Guest root permits unrelated identity'
        }
    }
}
# Gate above precedes all product operations. No SDK, network helper, or replacement installer here.
if ((Get-FileHash -LiteralPath 'C:\lab-input\windows-owned-settings.test.mjs' -Algorithm SHA256).Hash.ToLowerInvariant() -cne 'e7a659c90bce671329235b4434d59dcfa61f7ad72ba1b1f546c60b52534695d0') {
    throw 'Owned settings fixture digest differs; no product execution'
}
$channel = (Read-Bounded 'C:\lab-input\selected-channel.data-only.txt' 32).Trim()
if ($channel -cnotmatch '^(stable|main)$') { throw 'Owned selected channel invalid; no candidate execution' }
$target = Join-Path $work 'Owned Shell'; $project = Join-Path $work 'Project With Spaces'
$profile = Join-Path $work 'Simulated Guest Profile'
$receiptPath = Join-Path $work 'ui-acceptance.json'; $diagnosticPath = Join-Path $work 'ui-diagnostic.log'
$rawPath = Join-Path $work 'ui-terminal.private.bin'
$fixtureReport = Join-Path $work 'settings-fixtures.private.json'
$candidate = Join-Path $work 'candidate.private.exe'
$artifactPaths = @($candidate,$target,$project,$profile,$receiptPath,$diagnosticPath,$rawPath,$fixtureReport,"$work\fixture-before.private.txt","$work\fixture-after.private.txt")
foreach ($phase in @('before','inspect','installed','settings','gentle1','pi1','gentle2','pi2')) { $artifactPaths += "$work\$phase.cwd"; $artifactPaths += "$work\$phase.env" }
foreach ($path in $artifactPaths) {
    if (Test-Path -LiteralPath $path) { throw 'Fresh acceptance path exists; preserve it, refuse replacement' }
}
$result = [ordered]@{Schema='gentle-win11-ui/v1'; SourceBaseCommit=$control.SourceBaseCommit; SourceTree=$control.SourceTree; ProductSHA256=$control.ProductSHA256;
    CandidateExecuted=$false; Qualification=$true; InstallerTUI='unknown'; InstallerExit='unknown'; GentleUI='unknown'; PiUI='unknown';
    GentleExit='unknown'; PiExit='unknown'; CallerCWD='unknown'; CallerEnvironment='unknown'; CallerPATH='unknown'; ConsoleMode='unknown';
    Foreground='unknown'; FixturePreservation='unknown'; PersonalConfigurationPreservation='unknown'; OwnedCmdExit='unknown';
    ResourceChecks='unknown'; Cleanup='unknown'; CaptureComplete='unknown'; FunctionalReady=$false; Result='INCOMPLETE';
    GentleCtrlDReturn='unknown'; PiCtrlDReturn='unknown'; GentleExitMethod='unknown'; PiExitMethod='unknown'}
$provisionRenderedFailure = $null; $phaseTimings = $null; $fixtureSummary = $null; $mainIdentitySummary = $null
$result.EntryRuns = @(); $result.SelectedChannel = $channel
$terminal = $null; $failure = $null; $failurePhase = $null; $phaseContext = 'candidate-digest'; $before = $null
$oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $parentCwd = (Get-Location).Path
function Inventory([string[]]$roots) {
    $items = @(); $total = 0
    foreach ($root in $roots) {
        $pending = [Collections.Generic.Queue[string]]::new(); $pending.Enqueue($root)
        while ($pending.Count) {
            $directory = $pending.Dequeue()
            foreach ($item in Get-ChildItem -LiteralPath $directory -Force) {
                if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Fixture reparse point refused' }
                $items += ($item.FullName + '|D=' + $item.PSIsContainer)
                if ($items.Count -gt 512) { throw 'Whole inventory withheld: >512 entries' }
                if ($item.PSIsContainer) { $pending.Enqueue($item.FullName) } else {
                    $total += $item.Length; if ($total -gt 1048576) { throw 'Whole fixture inventory withheld: >1MiB' }
                    $items += ($item.FullName + '|' + $item.Length + '|' + (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash)
                }
            }
        }
    }
    $whole = ($items | Sort-Object) -join "`n"
    if ($utf8.GetByteCount($whole) -gt 65536) { throw 'Whole inventory withheld: >65536 bytes' }; return $whole
}
$marker = [Guid]::NewGuid().ToString('N')
function Boundary([string]$phase, [string]$command = 'ver > nul') {
    # Queue completion in cmd BEFORE launching UI; never type commands into the application's raw mode.
    # Nonce is expanded by cmd, not present literally in input. Delayed RC expansion runs after return.
    $terminal.Send("$command & echo G35_%G35MARK%_${phase}_RC=!errorlevel! & cd > `"$work\$phase.cwd`" & set > `"$work\$phase.env`" & echo G35_%G35MARK%_${phase}_DONE`r`n")
    $pattern = '(?m)^G35_' + $marker + '_' + $phase + '_DONE *$'
    return $pattern
}
try {
    $phaseContext = 'guest-dns-preflight'
    $result.GoProxyDNSResolved = [Net.Dns]::GetHostAddresses('proxy.golang.org').Length -gt 0
    if (-not $result.GoProxyDNSResolved) { throw 'Guest Go proxy DNS unavailable; no candidate execution' }
    if ($control.ProductSHA256 -notmatch '^[0-9a-f]{64}$' -or [long]$control.ProductBytes -le 0 -or [long]$control.ProductBytes -gt 67108864) { throw 'Incomplete product identity binding; no execution' }
    $sourceImage = 'C:\lab-input\candidate.inert.exe'
    if ((Get-Item -LiteralPath $sourceImage).Length -ne $control.ProductBytes -or (Get-FileHash -LiteralPath $sourceImage -Algorithm SHA256).Hash.ToLowerInvariant() -ne $result.ProductSHA256) { throw 'Inert candidate mismatch' }
    $phaseContext = 'private-guest-image-transfer'
    # Mapped input remains inert/read-only. The executing image inherits the normal
    # Guest's protected private NTFS ACL, rather than permissive mapping metadata.
    [IO.File]::Copy($sourceImage,$candidate,$false)
    if ((Get-Item -LiteralPath $candidate).Length -ne $control.ProductBytes -or (Get-FileHash -LiteralPath $candidate -Algorithm SHA256).Hash.ToLowerInvariant() -ne $result.ProductSHA256) { throw 'Private candidate copy mismatch; no execution' }
    $phaseContext = 'read-only-access-diagnostics'
    $trusted = @($identity.User.Value,'S-1-5-18','S-1-5-32-544','S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464')
    $objects = [ordered]@{Image=$candidate; InputDirectory=(Split-Path -Parent $candidate); SystemDrive=($env:SystemDrive+'\'); WindowsDirectory=$env:SystemRoot}
    $result.AccessReadbacks = @($objects.Keys | ForEach-Object {
        $acl = Get-Acl -LiteralPath $objects[$_]
        $sd = [Security.AccessControl.RawSecurityDescriptor]::new($acl.GetSecurityDescriptorBinaryForm(),0)
        $foreign = $false; $unsupported = $false; $foreignRules = @()
        foreach ($ace in $sd.DiscretionaryAcl) {
            if (([int]$ace.AceFlags -band 8) -ne 0 -or $ace.AceType -eq 1) { continue }
            if ($ace.AceType -ne 0) { $unsupported = $true; continue }
            if (($ace.AccessMask -band 0x500D0150) -ne 0 -and $trusted -notcontains $ace.SecurityIdentifier.Value) {
                $foreign = $true
                if ($foreignRules.Count -ge 8) { throw 'Whole foreign-rule diagnostic withheld: >8 rules' }
                $foreignRules += ('0x{0:X8}/flags={1}' -f $ace.AccessMask,[int]$ace.AceFlags)
            }
        }
        [ordered]@{Kind=$_; OwnerTrusted=($trusted -contains $sd.Owner.Value); DACLPresent=($null -ne $sd.DiscretionaryAcl); ForeignMutation=$foreign; UnsupportedRules=$unsupported; ForeignRules=($foreignRules -join '|')}
    }) # Bounded OS metadata only; no SID, paths, terminal text or private preimages exported.
    $phaseContext = 'fixture-preimages'
    New-Item -ItemType Directory -Path $project,(Join-Path $profile '.pi\agent') | Out-Null
    [IO.File]::WriteAllText((Join-Path $profile '.pi\agent\settings.json'),('{"fixtureMarker":"'+$marker+'"}'), $utf8)
    [IO.File]::WriteAllText((Join-Path $profile '.pi\agent\history.fixture'),$marker, $utf8)
    [IO.File]::WriteAllText((Join-Path $project 'project.fixture'),$marker, $utf8)
    $before = Inventory @($profile,$project)
    [IO.File]::WriteAllText("$work\fixture-before.private.txt",$before,$utf8)
    $env:HOME = $profile; $env:USERPROFILE = $profile # Explicit simulated profile, not real personal config evidence.
    $phaseContext = 'caller-terminal-start'
    $terminal = [GentleTerminal35]::new("$env:SystemRoot\System32\cmd.exe",$project)
    $terminal.Send("@echo off`r`nset G35MARK=$marker`r`n")
    $null = $terminal.Wait((Boundary 'before'),10000)
    $phaseContext = 'read-only-selection-inspection'
    $result.CandidateExecuted = 'unknown'
    $inspectPattern = Boundary 'inspect' "`"$candidate`" shell install --target `"$target`" --channel $channel --inspect"
    $inspectView = $terminal.Wait($inspectPattern,15000)
    $result.CandidateExecuted = $true
    if ($inspectView -match ('(?m)^G35_' + $marker + '_inspect_RC=([0-9]+) *$')) { $result.DirectInspectionExitCode = [uint32]$Matches[1] }
    $result.DirectInspectionPassed = $inspectView -match ('(?m)^G35_' + $marker + '_inspect_RC=0 *$')
    $clock = [Diagnostics.Stopwatch]::StartNew()
    $phaseContext = 'installer-header'
    $result.CandidateExecuted = 'unknown'; $installedPattern = Boundary 'installed' "`"$candidate`" shell install"
    $null = $terminal.Wait('Gentle Shell Windows 11 x64 user installer',20000); $result.CandidateExecuted = $true
    $phaseContext = 'installer-edit-review'
    $terminal.Send($target)
    $null = $terminal.Wait([regex]::Escape('Target: ' + $target),10000)
    if ($channel -ceq 'main') {
        # ConPTY's requested Win32 mode: two Tab presses, then Right on the channel field.
        $esc = [char]27; $channelFresh = $terminal.Mark()
        $tab = "$esc[9;15;9;1;0;1_$esc[9;15;9;0;0;1_"
        $terminal.Send($tab + $tab + "$esc[39;77;0;1;0;1_$esc[39;77;0;0;0;1_")
        $terminal.WaitFresh($channelFresh,'Channel: main',10000)
    } else { $null = $terminal.Wait('Channel: stable',10000) }
    $terminal.PressEnter()
    $null = $terminal.Wait('Confirm this physical selection and both command bindings\? y installs;',15000)
    $null = $terminal.Wait([regex]::Escape($target),1000)
    $null = $terminal.Wait('(?m)^[a-f0-9]{64} *$',1000); $result.InstallerTUI = $true
    $phaseContext = 'installer-confirmation-exit'
    $terminal.Send('y')
    $view = $terminal.Wait($installedPattern,[math]::Max(1,950000 - [int]$clock.ElapsedMilliseconds))
    if ($view -match ('(?m)^G35_' + $marker + '_installed_RC=([0-9]+) *$')) { $result.InstallerExitCode = [uint32]$Matches[1] }
    $result.InstallerExit = $view -match ('(?m)^G35_' + $marker + '_installed_RC=0 *$')
    if (-not $result.InstallerExit) { throw ('Actual installer exit was not zero; observed code: ' + $result.InstallerExitCode) }
    if ($channel -ceq 'main') {
        $phaseContext = 'installed-main-identity'
        $installed = Read-Bounded (Join-Path $target 'manifest.json') 4096 | ConvertFrom-Json
        if ($installed.MainCommit -cnotmatch '^[a-f0-9]{40}$' -or $installed.MainArchiveSHA -cnotmatch '^[a-f0-9]{64}$' -or
            $installed.LockSHA -cnotmatch '^[a-f0-9]{64}$' -or
            (Get-FileHash (Join-Path $target 'runtime/archives/main.zip') -Algorithm SHA256).Hash.ToLowerInvariant() -cne $installed.MainArchiveSHA -or
            (Get-FileHash (Join-Path $target 'source/package-lock.json') -Algorithm SHA256).Hash.ToLowerInvariant() -cne $installed.LockSHA) { throw 'Installed Main snapshot/lock identity mismatch' }
        $mainIdentitySummary = [ordered]@{commit=$installed.MainCommit;archiveSHA256=$installed.MainArchiveSHA;lockSHA256=$installed.LockSHA} | ConvertTo-Json -Compress
    }
    $phaseContext = 'settings-fixtures'
    $fixtureCommand = '"{0}\runtime\node\node.exe" "C:\lab-input\windows-owned-settings.test.mjs" "{0}\provision.mjs" "{1}"' -f $target,$fixtureReport
    $fixtureView = $terminal.Wait((Boundary 'settings' $fixtureCommand),15000)
    $fixtureSummary = Read-Bounded $fixtureReport 4096
    $fixtures = $fixtureSummary | ConvertFrom-Json
    $badOutcomes = @($fixtures.outcomes | Where-Object { $_.expected -cnotin @('accept','reject') -or $_.observed -cne $_.expected -or $_.infrastructure -eq $true -or $_.assertionCode })
    $result.SettingsFixturePassed = $fixtures.schema -ceq 'windows-owned-settings-fixtures/v2' -and $fixtures.total -eq 37 -and $fixtures.passed -eq 37 -and $fixtures.failed -eq 0 -and @($fixtures.outcomes).Count -eq 37 -and @($fixtures.outcomes.name | Sort-Object -Unique).Count -eq 37 -and $badOutcomes.Count -eq 0 -and $fixtureView -match ('(?m)^G35_' + $marker + '_settings_RC=0 *$')
    if (-not $result.SettingsFixturePassed) { throw 'Owned-settings fixture assertion failed; complete typed report retained' }
    foreach ($entry in @(@('gentle-shell','gentle1','Gentle'),@('pi','pi1','Pi'),@('gentle-shell','gentle2','Gentle'),@('pi','pi2','Pi'))) {
        $app = $entry[0]; $phase = $entry[1]; $property = $entry[2]
        $observation = [ordered]@{phase=$phase;opened=$false;exit0=$false}
        $result.EntryRuns += $observation
        $phaseContext = "$phase-opening"
        $clock.Restart(); $fresh = $terminal.Mark()
        # Bare names resolve in a nested current CMD only; parent PATH remains unchanged.
        $command = 'cmd /D /Q /V:ON /C "set "PATH={0}\bin;!PATH!" & call {1}"' -f $target,$app
        $returnPattern = Boundary $phase $command
        # Fresh-screen and distinct return nonces prevent earlier launches proving later ones.
        $terminal.WaitFresh($fresh,'Pi can explain its own features|clear/exit',180000)
        # One trusted OS snapshot: actual owned Node argv must identify this role.
        $processes = @(Get-CimInstance Win32_Process)
        if ($processes.Count -gt 512) { throw 'Whole process observation exceeds bound' }
        $owned = [Collections.Generic.HashSet[uint32]]::new(); $null = $owned.Add($terminal.Pid)
        for ($depth=0; $depth -lt 8; $depth++) {
            foreach ($p in $processes) { if ($owned.Contains([uint32]$p.ParentProcessId)) { $null = $owned.Add([uint32]$p.ProcessId) } }
        }
        $nodePath = Join-Path $target 'runtime\node\node.exe'
        $piPath = Join-Path $target 'prefix\node_modules\@earendil-works\pi-coding-agent\dist\bundle\cli.js'
        $shellPath = Join-Path $target 'prefix\node_modules\gentle-pi\bin\gentle-shell.mjs'
        # Fixed paths contain spaces and must be quoted as complete argv members.
        $nodePattern = '^(?i)"' + [regex]::Escape($nodePath) + '"\s+"'
        $piNodes = @($processes | Where-Object { $owned.Contains([uint32]$_.ProcessId) -and $_.ExecutablePath -ieq $nodePath -and $_.CommandLine -match ($nodePattern + [regex]::Escape($piPath) + '"(?:\s|$)') })
        $shellNodes = @($processes | Where-Object { $owned.Contains([uint32]$_.ProcessId) -and $_.ExecutablePath -ieq $nodePath -and $_.CommandLine -match ($nodePattern + [regex]::Escape($shellPath) + '"(?:\s|$)') })
        if ($piNodes.Count -ne 1 -or ($app -ceq 'pi' -and $shellNodes.Count -ne 0) -or ($app -ceq 'gentle-shell' -and $shellNodes.Count -ne 1)) { throw 'Live owned role process/argv not observed' }
        if ($app -ceq 'gentle-shell') {
            $parent = [uint32]$piNodes[0].ParentProcessId; $linked = $false
            for ($depth=0; $depth -lt 8; $depth++) {
                if ($parent -eq [uint32]$shellNodes[0].ProcessId) { $linked = $true; break }
                $ancestor = @($processes | Where-Object { [uint32]$_.ProcessId -eq $parent })
                if ($ancestor.Count -ne 1) { break }; $parent = [uint32]$ancestor[0].ParentProcessId
            }
            if (-not $linked) { throw 'Pi child does not descend from the owned Shell launcher' }
        }
        $observation.opened = $true; $result[($property+'UI')] = $true
        $result.InstallOpenPassed = $result.InstallerExit -ceq $true -and $result.GentleUI -ceq $true
        $phaseContext = "$phase-quit-command-return"
        $terminal.Send('/quit')
        $terminal.PressEnter()
        $view = $terminal.Wait($returnPattern,90000)
        $observation.exit0 = $view -match ('(?m)^G35_' + $marker + '_' + $phase + '_RC=0 *$')
        $result[($property+'ExitMethod')] = '/quit'; $result[($property+'Exit')] = $observation.exit0
        if (-not $observation.exit0) { throw 'Installed entry did not return zero; no repeated-entry acceptance' }
    }
    $result.RepeatedEntriesPassed = $result.EntryRuns.Count -eq 4 -and @($result.EntryRuns | Where-Object { -not $_.opened -or -not $_.exit0 }).Count -eq 0
    $phaseContext = 'caller-postimages'
    $result.CallerCWD = $true; $result.CallerEnvironment = $true; $result.CallerPATH = $true
    $baseline = Read-Bounded "$work\before.env" 65536
    foreach ($phase in @('before','installed','settings','gentle1','pi1','gentle2','pi2')) {
        if ((Read-Bounded "$work\$phase.cwd" 4096).TrimEnd("`r","`n") -cne $project) { $result.CallerCWD = $false }
        $environment = Read-Bounded "$work\$phase.env" 65536
        if ($environment -cne $baseline) { $result.CallerEnvironment = $false }
        if (($environment -split "`r?`n" | Where-Object {$_ -match '^Path='}) -cne ($baseline -split "`r?`n" | Where-Object {$_ -match '^Path='})) { $result.CallerPATH = $false }
    }
    $phaseContext = 'caller-exit-reap'
    $terminal.Send("exit /b 0`r`n"); $result.OwnedCmdExit = $terminal.Finish(5000) -eq 0
} catch { $failure = $_.Exception.Message; $failurePhase = $phaseContext } finally {
    if ($terminal) {
        # Semantic observation only: never export terminal text or private preimages.
        $result.Win32InputRequested = $terminal.Win32InputRequested
        $result.PiReturnMarkerObserved = $terminal.Matches('G35_' + $marker + '_pi[12]_RC=[0-9]+')
        $result.SettingsBindingRefusal = $terminal.Matches('owned package/settings bindings changed')
        $result.PiMissingModelsVisible = $terminal.Matches('No models available')
        try { $result.OwnedSettingsKeyNames = @((Read-Bounded (Join-Path $target 'agent/settings.json') 4096 | ConvertFrom-Json).PSObject.Properties.Name) } catch { $result.OwnedSettingsKeyNames = 'unverified' }
        $refusals = [ordered]@{Owner='foreign Windows owner'; ForeignMutation='mutation access'; CanonicalSelection='owned canonical Windows'; Reparse='aliases/reparse'; PhysicalAlias='drive/physical path alias'; FileSystem='requires local NTFS'; AccessRule='unsupported Windows access'; CanonicalDrive='canonical local drive'; DACL='private Windows DACL'; MissingOwner='missing Windows owner'}
        $result.SelectionRefusalCodes = @($refusals.Keys | Where-Object { $terminal.Matches($refusals[$_]) })
        $result.SelectionRefusalVisible = $result.SelectionRefusalCodes.Count -gt 0
        try {
            $stagesForProgress = @(Get-ChildItem -LiteralPath $work -Directory -Filter '.gentle-shell-windows-stage-*')
            if ($stagesForProgress.Count -eq 1 -and -not ($stagesForProgress[0].Attributes -band [IO.FileAttributes]::ReparsePoint)) {
                $phaseFiles = @('runtime/node/node.exe','runtime/go/bin/go.exe','agent/bin/rg.exe','agent/bin/fd.exe','provision.mjs','supervisor.exe','manifest.json')
                $result.StagePhaseFiles = ($phaseFiles | Where-Object { Test-Path -LiteralPath (Join-Path $stagesForProgress[0].FullName $_) }) -join '|'
            }
            $result.CompletedDestinationPresent = Test-Path -LiteralPath (Join-Path $target 'manifest.json')
        } catch { $result.StageProgressWhollyWithheld = $true }
        if ($result.SelectionRefusalCodes -contains 'CanonicalDrive') {
            try {
            # Read-only relative-name diagnosis after the owned subtree has settled.
            # Never export caller paths, contents, environment, or credential material.
            $stages = @(Get-ChildItem -LiteralPath $work -Directory -Filter '.gentle-shell-windows-stage-*')
            if ($stages.Count -eq 1 -and -not ($stages[0].Attributes -band [IO.FileAttributes]::ReparsePoint)) {
                $pending = [Collections.Generic.Queue[string]]::new(); $pending.Enqueue($stages[0].FullName)
                $visited = 0; $badNames = @(); $diagnosisComplete = $true
                while ($pending.Count -and $diagnosisComplete -and $badNames.Count -lt 2) {
                    foreach ($item in Get-ChildItem -LiteralPath $pending.Dequeue() -Force) {
                        $visited++
                        if ($visited -gt 32768 -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { $diagnosisComplete = $false; break }
                        $relative = $item.FullName.Substring($stages[0].FullName.Length + 1)
                        if ($relative.IndexOfAny([char[]]'%&^!') -ge 0) {
                            if ($utf8.GetByteCount($relative) -gt 256 -or $relative -notmatch '^[A-Za-z0-9_./\\@%&^!+ -]+$') { $diagnosisComplete = $false; break }
                            $badNames += $relative
                            if ($badNames.Count -eq 2) { break }
                        }
                        if ($item.PSIsContainer) { $pending.Enqueue($item.FullName) }
                    }
                }
                if ($diagnosisComplete) { $result.RuntimeMetaCharacterNames = $badNames -join '|' }
                else { $result.RuntimeNameDiagnosisWhollyWithheld = $true }
            }
            } catch { $result.RuntimeNameDiagnosisWhollyWithheld = $true }
        }
        $result.ExactTargetVisible = $terminal.Matches([regex]::Escape('Target: ' + $target))
        $result.SpaceStrippedTargetVisible = $terminal.Matches([regex]::Escape('Target: ' + $target.Replace(' ','')))
        $result.ReviewPromptVisible = $terminal.Matches('Confirm this physical selection')
        try { $terminal.Dispose(); $result.Cleanup = $true } catch { $result.Cleanup = $false; if (-not $failure) { $failure = $_.Exception.Message; $failurePhase = 'owned-terminal-cleanup' } }
        $raw = $terminal.WholeCapture(); $result.CaptureComplete = $null -ne $raw -and $result.Cleanup -eq $true
        $result.RawTerminalBytes = $terminal.RawBytes; $result.RawTerminalWhollyWithheld = $terminal.Withheld
        if ($null -ne $raw) {
            [IO.File]::WriteAllBytes($rawPath,$raw) # Raw terminal remains private in Guest.
            $phaseTimings = $terminal.PhaseLines()
            $provisionRenderedFailure = $terminal.RenderedFailure('Error: ', ('G35_' + $marker + '_installed_RC='),10000)
        }
    }
    try {
        if ($before) { $after = Inventory @($profile,$project); $result.FixturePreservation = $after -ceq $before; [IO.File]::WriteAllText("$work\fixture-after.private.txt",$after,$utf8) }
        $null = [GentleOSJob35]::VerifyCurrent(); $afterVolume = Get-CimInstance Win32_LogicalDisk -Filter "DeviceID='R:'"; $afterACL = Get-Acl -LiteralPath $work
        $result.ResourceChecks = $afterVolume.Size -eq $volume.Size -and $afterVolume.FileSystem -eq 'NTFS' -and $afterVolume.DriveType -eq 3 -and $afterACL.AreAccessRulesProtected -and $afterACL.GetOwner([Security.Principal.SecurityIdentifier]).Value -eq $identity.User.Value
    } catch { $result.ResourceChecks = $false; if (-not $failure) { $failure = $_.Exception.Message; $failurePhase = 'fixture-postimages-live-job' } }
    $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
    if ((Get-Location).Path -cne $parentCwd) { $result.CallerCWD = $false }
    # Exceptions describe harness operations, not raw UI capture. Never export raw UI/environment/preimages.
    $diagnostic = 'Stock UI authenticity, console modes and foreground restoration unproved. Fixture scope only.'
    if ($failure) {
        $result.FailurePhase = $failurePhase
        $result.FailureMessage = if ($utf8.GetByteCount($failure) -le 1024) { $failure } else { 'Whole exception message withheld (>1024 UTF-8 bytes)' }
        $diagnostic += ' Failure phase: ' + $failurePhase + '. Exception: ' + $result.FailureMessage
    }
    if ($fixtureSummary) { $diagnostic += "`nComplete typed owned-settings fixture report:`n" + $fixtureSummary }
    if ($mainIdentitySummary) { $diagnostic += "`nInstalled Main source and lock identity:`n" + $mainIdentitySummary }
    if ($phaseTimings) { $diagnostic += "`nFixed-name installer phase elapsed times:`n" + $phaseTimings }
    if ($provisionRenderedFailure) { $diagnostic += "`nComplete rendered installer error (not raw terminal/pipe):`n" + $provisionRenderedFailure }
    if ($utf8.GetByteCount($diagnostic) -le 12000) { [IO.File]::WriteAllText($diagnosticPath,$diagnostic,$utf8) }
    if ($failure -or @('InstallerExit','GentleExit','PiExit','CallerCWD','CallerEnvironment','CallerPATH','FixturePreservation','ResourceChecks','Cleanup','CaptureComplete','OwnedCmdExit' | Where-Object { $result[$_] -ceq $false }).Count) { $result.Result = 'FAIL' }
    $json = $result | ConvertTo-Json -Compress
    if ($utf8.GetByteCount($json) -gt 2700) { throw 'Whole receipt withheld: >2700-byte reserved acceptance envelope' }
    [IO.File]::WriteAllText($receiptPath,$json,$utf8)
}
