import os,pathlib,subprocess,json,re,time,hashlib,sys
p=pathlib.Path
assert sys.argv[1:]==['parent-green']
status=p('/proc/self/status').read_text()
assert os.getuid()==os.getgid()==1000 and os.getgroups()==[]
for n in ['CapInh','CapPrm','CapEff','CapBnd','CapAmb']:assert re.search('^'+n+r':\s+0+$',status,re.M),n
assert re.search(r'^NoNewPrivs:\s+1$',status,re.M)
member=p('/proc/self/cgroup').read_text().strip().removeprefix('0::')
group=p('/sys/fs/cgroup')/member.lstrip('/')
limits={n:(group/n).read_text().strip() for n in ['memory.max','memory.swap.max','cpu.max','pids.max']}
assert list(limits.values())==['3221225472','0','100000 100000','64']
assert os.statvfs('/tmp').f_blocks*os.statvfs('/tmp').f_frsize==2147483648
parent=p('/fixture/install-parent')
assert parent.is_dir() and parent.stat().st_mode&0o7777==0o700 and parent.stat().st_uid==1000
assert parent.stat().st_dev!=p('/tmp').stat().st_dev
assert not p('/usr/bin/node').exists()
env={'HOME':'/fixture/home','XDG_RUNTIME_DIR':'/run/user/1000','DBUS_SESSION_BUS_ADDRESS':'unix:path=/run/user/1000/bus','PATH':'/usr/bin:/bin','TERM':'dumb'}

import pty,termios,fcntl,struct,select,errno
mode=p('/probe/tui-mode').read_text().strip()
assert mode in ['navigation','actual']
master,slave=pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',70,160,0,0))
before=termios.tcgetattr(slave)
rawBefore=fcntl.ioctl(slave,termios.TCGETS,bytes(44))
raw=bytearray();phaseStart=0
report={'mode':mode,'command':['/probe/gentle-ai'],'limits':limits,'terminalBeforeLflag':before[3],'transitions':[],'selectedProfile':{'persona':'neutral','background_subagents':'off'},'actualInstallAuthorized':mode=='actual','terminalRestoredBeforeWorkspace':None}
started=time.monotonic()
def save():
 report['elapsedSeconds']=round(time.monotonic()-started,3)
 p('/fixture/tui-result.json').write_text(json.dumps(report,indent=2))
def childSetup():
 os.setsid();fcntl.ioctl(slave,termios.TIOCSCTTY,0)
env['INDEPENDENT_TUI_PROBE']='yes'
process=subprocess.Popen(['/probe/gentle-ai'],stdin=slave,stdout=slave,stderr=slave,env={**env,'TERM':'xterm-256color'},preexec_fn=childSetup)
expect={'PID':process.pid,'Flags':list(struct.unpack('IIII',rawBefore[:16])),'Cc':list(rawBefore[17:36]),'Dev':os.fstat(slave).st_dev,'Ino':os.fstat(slave).st_ino,'ConfirmNS':0}
p('/fixture/probe-expect.json').write_text(json.dumps(expect))
def strip(data):
 text=data.decode('utf-8','replace')
 text=re.sub(r'\x1b\][^\x07]*(?:\x07|\x1b\\)','',text)
 return re.sub(r'\x1b\[[0-?]*[ -/]*[@-~]','',text)
def readSome(wait=.05):
 ready,_,_=select.select([master],[],[],wait)
 if ready:
  try:data=os.read(master,65536)
  except OSError as e:
   if e.errno==errno.EIO:return
   raise
  if data:
   if len(raw)+len(data)>4194304:raise RuntimeError('bounded PTY output exceeded')
   raw.extend(data)
   with p('/fixture/tui-output.raw').open('ab') as stream:stream.write(data)
   for query,response in [(b'\x1b[6n',b'\x1b[1;1R'),(b'\x1b]11;?',b'\x1b]11;rgb:0000/0000/0000\x07'),(b'\x1b]10;?',b'\x1b]10;rgb:ffff/ffff/ffff\x07')]:
    if query in data and b'\x1b[?1049l' not in raw:os.write(master,response)
 if mode=='actual' and report.get('confirmed') and report['terminalRestoredBeforeWorkspace'] is None:
  work=list(parent.glob('.gentle-native-*'))
  if work:
   report['terminalRestoredBeforeWorkspace']=termios.tcgetattr(slave)==before
   report['workspaceFirstObservedSeconds']=round(time.monotonic()-started,3);save()
def waitFor(text,timeout=12):
 stop=time.monotonic()+timeout
 while time.monotonic()<stop:
  readSome()
  if text in strip(bytes(raw[phaseStart:])):
   report['transitions'].append({'view':text,'seconds':round(time.monotonic()-started,3),'lflag':termios.tcgetattr(slave)[3]});save();return
  if process.poll() is not None:raise RuntimeError('TUI exited before '+text)
 raise RuntimeError('TUI did not render '+text)
def send(text):
 global phaseStart
 phaseStart=len(raw);os.write(master,text.encode())
try:
 waitFor('Install Gentle-Shell, our own agent')
 send('\r');waitFor('Configure Gentle-Shell experience')
 if mode=='navigation':
  send('\x1b[B\r'+'\x1b[B'*2+'\r\x1b[F\r')
 else:
  send('\x1b[F\r')
 waitFor('Target:')
 send('/fixture/install-parent/final\r');waitFor('Ready — review before installation')
 if mode=='actual':
  send('\x05');waitFor('Configure Gentle-Shell experience')
  send('\x1b[B\r'+'\x1b[B'*2+'\r\x1b[F\r');waitFor('Target:')
  assert not (parent/'final').exists()
  send('\r');waitFor('Ready — review before installation')
 ready=strip(bytes(raw[phaseStart:]))
 assert 'Selected experience' in ready and 'Changes + scopes' in ready
 assert 'neutral' in ready and 'off' in ready
 report['readyTwoPanelsObserved']=True;report['terminalReadyLflag']=termios.tcgetattr(slave)[3]
 assert not (parent/'final').exists();save()
 if mode=='navigation':
  send('\x1b');waitFor('Install Gentle-Shell, our own agent');send('q')
 else:
  report['confirmed']=True;save()
  expect['ConfirmNS']=time.time_ns();p('/fixture/probe-expect.json').write_text(json.dumps(expect))
  send('y');waitFor('INERT_OWNER_READY')
  send('inherited-input\n');waitFor('INERT_OWNER_INPUT_OK')
  process.send_signal(__import__('signal').SIGTERM)
 stop=started+(25 if mode=='navigation' else 138)
 while process.poll() is None and time.monotonic()<stop:readSome(.1)
 if process.poll() is None:raise RuntimeError('bounded TUI driver deadline reached')
 for _ in range(3):readSome(.01)
 report['exitCode']=process.returncode
 report['workspaceTermiosObservationIsNotBubbleTeaVerdict']=True
 report['terminalAfterLflag']=termios.tcgetattr(slave)[3]
 report['terminalRestoredAfterExit']=termios.tcgetattr(slave)==before
 report['finalExists']=(parent/'final').exists()
 save()
 assert report['terminalRestoredAfterExit']
 if mode=='navigation':assert process.returncode==0 and not report['finalExists']
 else:
  proof=json.loads(p('/fixture/owner-proof.json').read_text())
  assert process.returncode==1 and not report['finalExists'] and proof['PID']==process.pid and proof['signalReceived']
  assert 'INHERITED_STDERR' in strip(bytes(raw)) and 'Error: INERT_OWNER_SIGNAL_RECEIVED' in strip(bytes(raw))
  report['ownerProof']=proof;report['observedAlternateScreenEntries']=bytes(raw).count(b'\x1b[?1049h')
  assert report['observedAlternateScreenEntries']==1
  save()
except BaseException as e:
 report['terminalRestoredAfterExitDiagnostic']=termios.tcgetattr(slave)==before
 report['rawAfterDiagnostic']=list(struct.unpack('IIII',fcntl.ioctl(slave,termios.TCGETS,bytes(44))[:16]))
 report['error']=repr(e);save()
 if process.poll() is None:
  process.terminate()
  try:process.wait(timeout=2)
  except subprocess.TimeoutExpired:process.kill();process.wait(timeout=2)
 raise
finally:
 os.close(master);os.close(slave)
print(json.dumps(report,indent=2),flush=True)
