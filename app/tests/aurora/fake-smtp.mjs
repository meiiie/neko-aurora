// Local fake SMTP (STARTTLS-only, AUTH PLAIN) for touch tests: the box blocks
// outbound SMTP, so delivery after approval is verified here. Captured mail is
// served on http://127.0.0.1:$HTTP/mails; auth secrets are reported only as a
// SHA-256 digest so tests never print them.
// Usage: node fake-smtp.mjs <certdir>  (expects cert.pem/key.pem for 127.0.0.1)
import net from 'node:net';import tls from 'node:tls';import http from 'node:http';import fs from 'node:fs';import crypto from 'node:crypto';
const dir=process.argv[2]||'/tmp/fake-smtp',PORT=+(process.env.SMTP_PORT||8925),HTTP=+(process.env.HTTP_PORT||8926);
const ctx={key:fs.readFileSync(dir+'/key.pem'),cert:fs.readFileSync(dir+'/cert.pem')};const mails=[];
net.createServer(sock=>{let s=sock,buf='',data=null,tlsOn=false,cur={};const say=x=>s.write(x+'\r\n');
 const onLine=line=>{if(data!==null){if(line==='.'){cur.data=data;mails.push(cur);cur={user:cur.user,pwSha:cur.pwSha};data=null;say('250 queued')}else data+=line+'\n';return}
  const cmd=line.split(' ')[0].toUpperCase();
  if(cmd==='EHLO'||cmd==='HELO')say(tlsOn?'250-fake\r\n250 AUTH PLAIN':'250-fake\r\n250 STARTTLS');
  else if(cmd==='STARTTLS'){say('220 go');s.removeAllListeners('data');const t=new tls.TLSSocket(s,{isServer:true,...ctx});s=t;tlsOn=true;buf='';t.on('data',feed);t.on('error',()=>{})}
  else if(cmd==='AUTH'){if(!tlsOn){say('530 tls first');return}const p=Buffer.from(line.split(' ')[2]||'','base64').toString().split('\0');cur.user=p[1];cur.pwSha=crypto.createHash('sha256').update(p[2]||'').digest('hex');say('235 ok')}
  else if(cmd==='MAIL'){cur.from=line.slice(10).replace(/[<>]/g,'');say('250 ok')}else if(cmd==='RCPT'){cur.to=line.slice(8).replace(/[<>]/g,'');say('250 ok')}
  else if(cmd==='DATA'){data='';say('354 go')}else if(cmd==='QUIT'){say('221 bye');s.end()}else say('250 ok')};
 const feed=c=>{buf+=c.toString();let i;while((i=buf.indexOf('\r\n'))>=0){const l=buf.slice(0,i);buf=buf.slice(i+2);onLine(l)}};
 sock.on('data',feed);sock.on('error',()=>{});say('220 fake ESMTP')}).listen(PORT,'127.0.0.1',()=>console.log('fake smtp on',PORT));
http.createServer((q,r)=>{if(q.url==='/reset'){mails.length=0}r.writeHead(200,{'content-type':'application/json'});r.end(JSON.stringify(mails))}).listen(HTTP,'127.0.0.1');
