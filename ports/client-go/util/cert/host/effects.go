// Copyright 2026 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0.
// External crypto, filesystem and TLS primitives. Port policy lives in .gfx.
package host

import (
 "context"
 "crypto"
 "crypto/ecdsa"
 "crypto/elliptic"
 "crypto/rand"
 "crypto/rsa"
 "crypto/tls"
 "crypto/x509"
 "crypto/x509/pkix"
 "encoding/pem"
 "errors"
 "fmt"
 "math/big"
 "net"
 "net/url"
 "os"
 "path/filepath"
 "reflect"
 "time"
 "gofx/rt"
 "github.com/reoring/gofx/ports/client-go/host/deps"
 netutils "k8s.io/utils/net"
 apierrors "k8s.io/apimachinery/pkg/api/errors"
)
func effect(f func(context.Context)(any,error)) any { return rt.IO(func(ctx context.Context)(any,error){if e:=ctx.Err();e!=nil{return nil,e};return f(ctx)}) }
func value[T any](ctx context.Context,v any) T {t:=reflect.TypeFor[T]();if t.Kind()==reflect.Interface {if _,ok:=v.(hostdeps.Value);ok{v=rt.SomeOf(v)}};var result T;reflect.ValueOf(&result).Elem().Set(hostdeps.To(t,v,ctx));return result}
func external(v any) any{return hostdeps.From(reflect.ValueOf(v))}
func iface[T any](v T)any {r:=hostdeps.From(reflect.ValueOf(&v).Elem());if x,ok:=rt.OptionValue(r);ok{return x};return r}
func errValue(e error)any{if e==nil{return rt.NoneOf()};return rt.SomeOf(iface[error](e))}
func pair(v any,e error)any{return rt.Tuple2Of(v,errValue(e))}
func list(xs []any)any{r:=rt.Shapes.Nil();for i:=len(xs)-1;i>=0;i--{r=rt.Shapes.Cons(xs[i],r)};return r}
func Error(s any)any{return iface[error](errors.New(s.(string)))}
func Wrap(s,e any)any{return iface[error](fmt.Errorf("%s: %w",s.(string),value[error](context.Background(),e)))}
func ErrorText(e any)any{return value[error](context.Background(),e).Error()}
func Quote(s any)any{return fmt.Sprintf("%q",s.(string))}
func IsNotExist(e any)any{return os.IsNotExist(value[error](context.Background(),e))}
func APIErrorKind(e any)any{v:=value[error](context.Background(),e);switch{case apierrors.IsAlreadyExists(v):return "AlreadyExists";case apierrors.IsNotFound(v):return "NotFound";case apierrors.IsUnauthorized(v):return "Unauthorized";case apierrors.IsForbidden(v):return "Forbidden";case apierrors.IsUnexpectedServerError(v):return "UnexpectedServerError"};return "Other"}
func FormatAPIError(prefix,e any)any{v:=value[error](context.Background(),e);if s,ok:=v.(apierrors.APIStatus);ok{status:=s.Status();status.Message=prefix.(string)+status.Message;return iface[error](&apierrors.StatusError{ErrStatus:status})};return iface[error](fmt.Errorf("%s%v",prefix.(string),v))}
func ReadFile(p any)any{return effect(func(context.Context)(any,error){b,e:=os.ReadFile(p.(string));return pair(string(b),e),nil})}
func WriteFile(p,b,mode any)any{return effect(func(context.Context)(any,error){return errValue(os.WriteFile(p.(string),[]byte(b.(string)),os.FileMode(mode.(int64)))),nil})}
func Mkdir(p any)any{return effect(func(context.Context)(any,error){return errValue(os.MkdirAll(p.(string),0755)),nil})}
func Parent(p any)any{return filepath.Dir(p.(string))}
func Join(a,b any)any{return filepath.Join(a.(string),b.(string))}
func Absolute(p any)any{return effect(func(context.Context)(any,error){s,e:=filepath.Abs(p.(string));return pair(s,e),nil})}
func Readable(p any)any{return effect(func(context.Context)(any,error){f,e:=os.Open(p.(string));if e!=nil{return false,nil};_ = f.Close();return true,nil})}
func Stat(p,follow any)any{return effect(func(context.Context)(any,error){var s os.FileInfo;var e error;if follow.(bool){s,e=os.Stat(p.(string))}else{s,e=os.Lstat(p.(string))};if os.IsNotExist(e){return pair(int64(0),nil),nil};if e!=nil{return pair(int64(0),e),nil};if s.Mode()&os.ModeSymlink!=0{return pair(int64(2),nil),nil};return pair(int64(1),nil),nil})}
func Remove(p any)any{return effect(func(context.Context)(any,error){return errValue(os.Remove(p.(string))),nil})}
func Rename(a,b any)any{return effect(func(context.Context)(any,error){return errValue(os.Rename(a.(string),b.(string))),nil})}
func Symlink(a,b any)any{return effect(func(context.Context)(any,error){return errValue(os.Symlink(a.(string),b.(string))),nil})}
func DecodePEM(b any)any {block,rest:=pem.Decode([]byte(b.(string)));if block==nil{return rt.NoneOf()};return rt.SomeOf(rt.Tuple2Of(block.Type,rt.Tuple2Of(int64(len(block.Headers)),rt.Tuple2Of(string(block.Bytes),string(rest)))))}
func EncodePEM(t,b any)any{return string(pem.EncodeToMemory(&pem.Block{Type:t.(string),Bytes:[]byte(b.(string))}))}
func ParsePrivate(format,b any)any{return effect(func(context.Context)(any,error){var k any;var e error;switch format.(string){case "EC PRIVATE KEY":k,e=x509.ParseECPrivateKey([]byte(b.(string)));case "RSA PRIVATE KEY":k,e=x509.ParsePKCS1PrivateKey([]byte(b.(string)));case "PRIVATE KEY":k,e=x509.ParsePKCS8PrivateKey([]byte(b.(string)));default:e=errors.New("unknown private key format")};if e!=nil{return pair(rt.NoneOf(),e),nil};return pair(rt.SomeOf(iface[crypto.PrivateKey](k)),nil),nil})}
func ParsePublic(format,b any)any{return effect(func(context.Context)(any,error){var k any;var e error;switch format.(string){case "PKIX":k,e=x509.ParsePKIXPublicKey([]byte(b.(string)));case "CERTIFICATE":var c *x509.Certificate;c,e=x509.ParseCertificate([]byte(b.(string)));if e==nil{k=c.PublicKey};case "RSA PRIVATE KEY":var p *rsa.PrivateKey;p,e=x509.ParsePKCS1PrivateKey([]byte(b.(string)));if e==nil{k=&p.PublicKey};case "EC PRIVATE KEY":var p *ecdsa.PrivateKey;p,e=x509.ParseECPrivateKey([]byte(b.(string)));if e==nil{k=&p.PublicKey};case "PRIVATE KEY":var p any;p,e=x509.ParsePKCS8PrivateKey([]byte(b.(string)));if e==nil{if s,ok:=p.(*rsa.PrivateKey);ok{k=&s.PublicKey}else{e=errors.New("not an RSA private key")}}};if e==nil{switch k.(type){case *rsa.PublicKey,*ecdsa.PublicKey:default:e=errors.New("not an RSA or ECDSA public key")}};if e!=nil{return pair(rt.NoneOf(),e),nil};return pair(rt.SomeOf(iface[crypto.PublicKey](k)),nil),nil})}
func MarshalPrivate(k any)any{return effect(func(ctx context.Context)(any,error){var b []byte;var t string;var e error;switch key:=value[crypto.PrivateKey](ctx,k).(type){case *rsa.PrivateKey:b=x509.MarshalPKCS1PrivateKey(key);t="RSA PRIVATE KEY";case *ecdsa.PrivateKey:b,e=x509.MarshalECPrivateKey(key);t="EC PRIVATE KEY";default:e=fmt.Errorf("private key is not a recognized type: %T",key)};if e!=nil{return pair("",e),nil};return pair(string(pem.EncodeToMemory(&pem.Block{Type:t,Bytes:b})),nil),nil})}
func GenerateEC()any{return effect(func(context.Context)(any,error){k,e:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader);return pair(rt.SomeOf(iface[crypto.PrivateKey](k)),e),nil})}
func GenerateRSA(bits any)any{return effect(func(context.Context)(any,error){k,e:=rsa.GenerateKey(rand.Reader,int(bits.(int64)));if e!=nil{return pair(rt.NoneOf(),e),nil};return pair(rt.SomeOf(iface[crypto.PrivateKey](k)),nil),nil})}
func KeyInfo(k any)any{return effect(func(ctx context.Context)(any,error){switch k:=value[crypto.PrivateKey](ctx,k).(type){case *rsa.PrivateKey:return rt.Tuple2Of("RSA",rt.Tuple2Of(int64(k.N.BitLen()),"")),nil;case *ecdsa.PrivateKey:return rt.Tuple2Of("ECDSA",rt.Tuple2Of(int64(k.Curve.Params().BitSize),k.Curve.Params().Name)),nil;default:return rt.Tuple2Of("Other",rt.Tuple2Of(int64(0),"")),nil}})}
func ParseCertificate(b any)any{return effect(func(context.Context)(any,error){c,e:=x509.ParseCertificate([]byte(b.(string)));return pair(external(c),e),nil})}
func CertificateDER(c any)any{return effect(func(ctx context.Context)(any,error){return string(value[*x509.Certificate](ctx,rt.SomeOf(c)).Raw),nil})}
func NewPool()any{return effect(func(context.Context)(any,error){v,_:=rt.OptionValue(external(x509.NewCertPool()));return v,nil})}
func AddCert(p,c any)any{return effect(func(ctx context.Context)(any,error){value[*x509.CertPool](ctx,rt.SomeOf(p)).AddCert(value[*x509.Certificate](ctx,rt.SomeOf(c)));return rt.Unit{},nil})}
func ParseCSR(b any)any{return effect(func(context.Context)(any,error){v,e:=x509.ParseCertificateRequest([]byte(b.(string)));return pair(external(v),e),nil})}
func MakeCSR(k,t,algorithm any)any{return effect(func(ctx context.Context)(any,error){template:=*value[*x509.CertificateRequest](ctx,rt.SomeOf(t));template.SignatureAlgorithm=x509.SignatureAlgorithm(algorithm.(int64));b,e:=x509.CreateCertificateRequest(rand.Reader,&template,value[crypto.PrivateKey](ctx,k));return pair(string(b),e),nil})}
func CSRTemplate(subject,dns,ips any)any{return effect(func(ctx context.Context)(any,error){t:=&x509.CertificateRequest{Subject:value[pkix.Name](ctx,subject),DNSNames:value[[]string](ctx,rt.SomeOf(dns)),IPAddresses:value[[]net.IP](ctx,rt.SomeOf(ips))};v,_:=rt.OptionValue(external(t));return v,nil})}
func SubjectEqual(a,b any)any{return effect(func(ctx context.Context)(any,error){return reflect.DeepEqual(value[*x509.CertificateRequest](ctx,rt.SomeOf(a)).Subject,value[*x509.CertificateRequest](ctx,rt.SomeOf(b)).Subject),nil})}
func TemplateEqual(a,b any)any{return effect(func(ctx context.Context)(any,error){return reflect.DeepEqual(value[*x509.CertificateRequest](ctx,a),value[*x509.CertificateRequest](ctx,b)),nil})}
func VerifyCSRKey(t,k any)any{return effect(func(ctx context.Context)(any,error){copy:=*value[*x509.CertificateRequest](ctx,rt.SomeOf(t));s,ok:=value[crypto.PrivateKey](ctx,k).(crypto.Signer);if !ok{return errValue(errors.New("privateKey is not a signer")),nil};copy.PublicKey=s.Public();return errValue(copy.CheckSignature()),nil})}
func KeyPair(c,k any)any{return effect(func(context.Context)(any,error){p,e:=tls.X509KeyPair([]byte(c.(string)),[]byte(k.(string)));if e!=nil{return pair(rt.NoneOf(),e),nil};p.Leaf,e=x509.ParseCertificate(p.Certificate[0]);return pair(external(&p),e),nil})}
func Leaf(c any)any{return effect(func(ctx context.Context)(any,error){return external(value[*tls.Certificate](ctx,rt.SomeOf(c)).Leaf),nil})}
func Times(c any)any{return effect(func(ctx context.Context)(any,error){v:=value[*x509.Certificate](ctx,rt.SomeOf(c));return rt.Tuple2Of(v.NotBefore.UnixNano(),v.NotAfter.UnixNano()),nil})}
func Names(c,request any)any{return effect(func(ctx context.Context)(any,error){var s pkix.Name;var dns []string;var ips []net.IP;if request.(bool){v:=value[*x509.CertificateRequest](ctx,rt.SomeOf(c));s=v.Subject;dns=v.DNSNames;ips=v.IPAddresses}else{v:=value[*x509.Certificate](ctx,rt.SomeOf(c));s=v.Subject;dns=v.DNSNames;ips=v.IPAddresses};ip:=make([]string,len(ips));for i,v:=range ips{ip[i]=v.String()};return rt.Tuple2Of(s.CommonName,rt.Tuple2Of(rt.ListFromStrings(s.Organization),rt.Tuple2Of(rt.ListFromStrings(dns),rt.ListFromStrings(ip)))),nil})}
func Timestamp(n any)any{return time.Unix(0,n.(int64)).Format("2006-01-02-15-04-05")}
func TimeText(n any)any{return time.Unix(0,n.(int64)).String()}
func RandomFraction()any{return effect(func(context.Context)(any,error){n,e:=rand.Int(rand.Reader,big.NewInt(1000000));if e!=nil{return nil,e};return n.Int64(),nil})}
func Serial()any{return effect(func(context.Context)(any,error){n,e:=rand.Int(rand.Reader,big.NewInt(9223372036854775806));if e!=nil{return nil,e};return n.Int64()+1,nil})}
func SignCertificate(subject,org,dns,ips,before,after,isCA,serial,key,parent,parentKey,keyUsage,extKeyUsage,basicConstraintsValid any)any{return effect(func(ctx context.Context)(any,error){t:=&x509.Certificate{SerialNumber:big.NewInt(serial.(int64)),Subject:pkix.Name{CommonName:subject.(string),Organization:value[[]string](ctx,rt.SomeOf(org))},DNSNames:value[[]string](ctx,rt.SomeOf(dns)),IPAddresses:value[[]net.IP](ctx,rt.SomeOf(ips)),NotBefore:time.Unix(0,before.(int64)).UTC(),NotAfter:time.Unix(0,after.(int64)).UTC(),IsCA:isCA.(bool),BasicConstraintsValid:basicConstraintsValid.(bool),KeyUsage:value[x509.KeyUsage](ctx,keyUsage),ExtKeyUsage:value[[]x509.ExtKeyUsage](ctx,rt.SomeOf(extKeyUsage))};p:=value[*x509.Certificate](ctx,parent);if p==nil{p=t};signer,ok:=value[crypto.PrivateKey](ctx,key).(crypto.Signer);if !ok{return pair(rt.NoneOf(),errors.New("privateKey is not a signer")),nil};b,e:=x509.CreateCertificate(rand.Reader,t,p,signer.Public(),value[crypto.PrivateKey](ctx,parentKey));if e!=nil{return pair(rt.NoneOf(),e),nil};c,e:=x509.ParseCertificate(b);return pair(external(c),e),nil})}
func ParseIP(s any)any{ip:=netutils.ParseIPSloppy(s.(string));if ip==nil{return rt.NoneOf()};return rt.SomeOf(external(ip))}
func URLHost(s any)any{u,e:=url.Parse(s.(string));if e!=nil{return pair("",e)};return pair(u.Host,nil)}
func Inspect(address,config any)any{return effect(func(ctx context.Context)(any,error){c:=value[*tls.Config](ctx,rt.SomeOf(config)).Clone();var names []string;c.GetClientCertificate=func(info *tls.CertificateRequestInfo)(*tls.Certificate,error){names=names[:0];for _,n:=range info.AcceptableCAs{names=append(names,string(n))};return &tls.Certificate{},nil};d:=tls.Dialer{Config:c};conn,e:=d.DialContext(ctx,"tcp",address.(string));if e!=nil{return pair(rt.Tuple2Of(rt.Shapes.Nil(),rt.Shapes.Nil()),e),nil};t:=conn.(*tls.Conn);certs:=t.ConnectionState().PeerCertificates;e=conn.Close();out:=make([]any,len(certs));for i,v:=range certs{x,_:=rt.OptionValue(external(v));out[i]=x};return pair(rt.Tuple2Of(rt.ListFromStrings(names),list(out)),e),nil})}
type NoCertKeyError string
func(e *NoCertKeyError)Error()string{return string(*e)}
func NoKeyError(s any)any{e:=NoCertKeyError(s.(string));return iface[error](&e)}
func IsNoKeyError(e any)any{var target *NoCertKeyError;return errors.As(value[error](context.Background(),e),&target)}
func FirstPEM(data any)any{b,_:=pem.Decode([]byte(data.(string)));if b==nil{return rt.NoneOf()};return rt.SomeOf(string(pem.EncodeToMemory(b)))}
func Seconds(n any)any{return float64(n.(int64))/float64(time.Second)}
func SubjectDifference(a,b any)any{return fmt.Sprintf("csr subjects differ: new: %#v, orig: %#v",value[*x509.CertificateRequest](context.Background(),rt.SomeOf(a)).Subject,value[*x509.CertificateRequest](context.Background(),rt.SomeOf(b)).Subject)}
func IPStrings(ips any)any{values:=value[[]net.IP](context.Background(),rt.SomeOf(ips));out:=make([]string,len(values));for i,ip:=range values{out[i]=ip.String()};return rt.ListFromStrings(out)}
func OpenWrite(path,mode any)any{return effect(func(context.Context)(any,error){f,e:=os.OpenFile(path.(string),os.O_CREATE|os.O_TRUNC|os.O_RDWR,os.FileMode(mode.(int64)));return pair(external(f),e),nil})}
func WriteOpened(f,b any)any{return effect(func(ctx context.Context)(any,error){_,e:=value[*os.File](ctx,rt.SomeOf(f)).Write([]byte(b.(string)));return errValue(e),nil})}
func SyncOpened(f any)any{return effect(func(ctx context.Context)(any,error){return errValue(value[*os.File](ctx,rt.SomeOf(f)).Sync()),nil})}
func CloseOpened(f any)any{return effect(func(ctx context.Context)(any,error){return errValue(value[*os.File](ctx,rt.SomeOf(f)).Close()),nil})}
func FileSyncError(path,e any)any{return iface[error](fmt.Errorf("failed to sync certificate data to disk: %w (file: %q)",value[error](context.Background(),e),path.(string)))}
