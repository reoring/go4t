// Copyright 2026 The Kubernetes Authors. Licensed under Apache-2.0.
package testsupport
import (
 "context"
 "crypto/ecdsa"
 "crypto/elliptic"
 "crypto/rand"
 "crypto/tls"
 "crypto/x509"
 "crypto/x509/pkix"
 "encoding/pem"
 "math/big"
 "net"
 "os"
 "reflect"
 "time"
 "sync"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 keys "github.com/reoring/gofx/ports/client-go/util/keyutil/testsupport"
)
func TempDir()any{return rt.IO(func(context.Context)(any,error){return os.MkdirTemp("","gofx-cert-fixture-")})}
func RemoveAll(p any)any{return rt.IO(func(context.Context)(any,error){return rt.Unit{},os.RemoveAll(p.(string))})}
func Mode(p any)any{return rt.IO(func(context.Context)(any,error){s,e:=os.Stat(p.(string));if e!=nil{return nil,e};return int64(s.Mode().Perm()),nil})}
func Readlink(p any)any{return rt.IO(func(context.Context)(any,error){s,e:=os.Readlink(p.(string));return s,e})}
func Pair(cn,before,after any)any{return rt.IO(func(context.Context)(any,error){keyPEM:=keys.Fixture("ec").(string);block,_:=pem.Decode([]byte(keyPEM));key,e:=x509.ParseECPrivateKey(block.Bytes);if e!=nil{return nil,e};template:=x509.Certificate{SerialNumber:big.NewInt(1),Subject:pkix.Name{CommonName:cn.(string),Organization:[]string{"fixture-org"}},DNSNames:[]string{"fixture.local"},NotBefore:time.Unix(0,before.(int64)),NotAfter:time.Unix(0,after.(int64)),KeyUsage:x509.KeyUsageDigitalSignature,ExtKeyUsage:[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},BasicConstraintsValid:true};b,e:=x509.CreateCertificate(rand.Reader,&template,&template,&key.PublicKey,key);if e!=nil{return nil,e};return rt.Tuple2Of(string(pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:b})),keyPEM),nil})}
func Template(cn any)any{return rt.IO(func(context.Context)(any,error){t:=&x509.CertificateRequest{Subject:pkix.Name{CommonName:cn.(string),Organization:[]string{"fixture-org"}},DNSNames:[]string{"fixture.local"}};v:=hostdeps.From(reflect.ValueOf(t));v,_=rt.OptionValue(v);return v,nil})}
func OtherKey()any{return rt.IO(func(context.Context)(any,error){key,e:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader);if e!=nil{return nil,e};b,e:=x509.MarshalECPrivateKey(key);if e!=nil{return nil,e};return string(pem.EncodeToMemory(&pem.Block{Type:"EC PRIVATE KEY",Bytes:b})),nil})}
type inspectionServer struct{listener net.Listener; mutex sync.Mutex; connections map[net.Conn]struct{}; workers sync.WaitGroup; completed chan bool}
func InspectionServer(certPEM,keyPEM any)any{return rt.IO(func(context.Context)(any,error){
 cert,e:=tls.X509KeyPair([]byte(certPEM.(string)),[]byte(keyPEM.(string)));if e!=nil{return nil,e}
 roots:=x509.NewCertPool();roots.AppendCertsFromPEM([]byte(certPEM.(string)))
 listener,e:=tls.Listen("tcp","127.0.0.1:0",&tls.Config{Certificates:[]tls.Certificate{cert},ClientAuth:tls.RequestClientCert,ClientCAs:roots});if e!=nil{return nil,e}
 server:=&inspectionServer{listener:listener,connections:make(map[net.Conn]struct{}),completed:make(chan bool,16)}
 server.workers.Add(1)
 go func(){defer server.workers.Done();for{conn,e:=listener.Accept();if e!=nil{return};server.mutex.Lock();server.connections[conn]=struct{}{};server.workers.Add(1);server.mutex.Unlock();go func(){defer server.workers.Done();defer conn.Close();c:=conn.(*tls.Conn);e:=c.Handshake();server.completed<-e==nil&&len(c.ConnectionState().PeerCertificates)==0;server.mutex.Lock();delete(server.connections,conn);server.mutex.Unlock()}()}}()
 return rt.Tuple2Of(listener.Addr().String(),server),nil
})}
func StopInspectionServer(value any)any{return rt.IO(func(context.Context)(any,error){server:=value.(*inspectionServer);server.listener.Close();server.mutex.Lock();for conn:=range server.connections{conn.Close()};server.mutex.Unlock();server.workers.Wait();return rt.Unit{},nil})}
func NoClientCredentials(value,count any)any{return rt.IO(func(ctx context.Context)(any,error){server:=value.(*inspectionServer);for i:=int64(0);i<count.(int64);i++{select{case good:=<-server.completed:if !good{return false,nil};case <-ctx.Done():return nil,ctx.Err()}};return true,nil})}
