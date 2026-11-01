#!/usr/bin/env python3
"""Emit explicit ordered external API registrations in native go4t."""
import re
from generate import Module,HEAD,save
from resources import DEFAULT_SOURCE

def generate():
 source=(DEFAULT_SOURCE/'kubernetes/scheme/register.go').read_text()
 aliases=dict(re.findall(r'^\s*(\w+)\s+"(k8s.io/api/[^"]+)"',source,re.M))
 order=re.findall(r'^\s*(\w+)\.AddToScheme,',source,re.M)
 m=Module('scheme')
 for a,p in [('runtime','deps/k8s.io/apimachinery/pkg/runtime'),('serializer','deps/k8s.io/apimachinery/pkg/runtime/serializer'),('features','features'),('rest','rest'),('errors','deps/support')]:m.imp(p,a)
 m.code.append('type Registry = Registry { scheme:runtime.Scheme,codecs:serializer.CodecFactory,parameterCodec:runtime.ParameterCodec,negotiatedSerializer:runtime.NegotiatedSerializer,decoder:runtime.Decoder }')
 host=HEAD+'package host\nimport ("context";"reflect";"go4t/rt";deps "github.com/reoring/gofx/ports/client-go/host/deps";"k8s.io/apimachinery/pkg/runtime";"k8s.io/apimachinery/pkg/runtime/serializer";meta "k8s.io/apimachinery/pkg/apis/meta/v1";"k8s.io/apimachinery/pkg/runtime/schema";"k8s.io/apimachinery/pkg/fields"\n'+''.join(a+' "'+aliases[a]+'"\n' for a in order)+')\n'
 host+='''func box[T any](v T) any { value:=deps.From(reflect.ValueOf(&v).Elem()); if raw,ok:=rt.OptionValue(value);ok{return raw};return value }
func schemeOf(value any,ctx context.Context)*runtime.Scheme{return deps.To(reflect.TypeFor[*runtime.Scheme](),rt.SomeOf(value),ctx).Interface().(*runtime.Scheme)}
func NewScheme() any{return rt.IO(func(ctx context.Context)(any,error){return box(runtime.NewScheme()),nil})}
func AddMeta(value any) any{return rt.IO(func(ctx context.Context)(any,error){meta.AddToGroupVersion(schemeOf(value,ctx),schema.GroupVersion{Version:"v1"});return rt.Unit{},nil})}
func Codecs(value any) any{return rt.IO(func(ctx context.Context)(any,error){return box(serializer.NewCodecFactory(schemeOf(value,ctx))),nil})}
func Parameters(value any) any{return rt.IO(func(ctx context.Context)(any,error){return box[runtime.ParameterCodec](runtime.NewParameterCodec(schemeOf(value,ctx))),nil})}
func WithoutConversion(value any) any{return rt.IO(func(ctx context.Context)(any,error){v:=deps.To(reflect.TypeFor[serializer.CodecFactory](),value,ctx).Interface().(serializer.CodecFactory);return box[runtime.NegotiatedSerializer](v.WithoutConversion()),nil})}
func Decoder(value any) any{return rt.IO(func(ctx context.Context)(any,error){v:=deps.To(reflect.TypeFor[serializer.CodecFactory](),value,ctx).Interface().(serializer.CodecFactory);return box[runtime.Decoder](v.UniversalDecoder()),nil})}
func FieldSelector(values any) any{return rt.IO(func(ctx context.Context)(any,error){set:=fields.Set{};for{pair,tail,ok:=rt.Shapes.Uncons(values);if !ok{break};k,v:=rt.Shapes.Untuple2(pair);set[k.(string)]=v.(string);values=tail};return box[fields.Selector](set.AsSelector()),nil})}
'''
 for a in order:
  name='add'+a[0].upper()+a[1:]
  m.code.append(f'extern func {name}(scheme runtime.Scheme) IO[Unit] = "github.com/reoring/gofx/ports/client-go/kubernetes/scheme/host".{name[0].upper()+name[1:]}')
  host+=f'func {name[0].upper()+name[1:]}(value any) any {{return rt.IO(func(ctx context.Context)(any,error){{return rt.Unit{{}},{a}.AddToScheme(schemeOf(value,ctx))}})}}\n'
 m.code.extend(['extern func addMeta(scheme runtime.Scheme) IO[Unit] = "github.com/reoring/gofx/ports/client-go/kubernetes/scheme/host".AddMeta','extern func codecs(scheme runtime.Scheme) IO[serializer.CodecFactory] = "github.com/reoring/gofx/ports/client-go/kubernetes/scheme/host".Codecs','extern func parameters(scheme runtime.Scheme) IO[runtime.ParameterCodec] = "github.com/reoring/gofx/ports/client-go/kubernetes/scheme/host".Parameters','extern func withoutConversion(codecs serializer.CodecFactory) IO[runtime.NegotiatedSerializer] = "github.com/reoring/gofx/ports/client-go/kubernetes/scheme/host".WithoutConversion','extern func decoder(codecs serializer.CodecFactory) IO[runtime.Decoder] = "github.com/reoring/gofx/ports/client-go/kubernetes/scheme/host".Decoder'])
 m.code.append('func addToScheme(scheme runtime.Scheme) IO[Unit] { do {\n'+''.join(' add'+a[0].upper()+a[1:]+'(scheme)\n' for a in order)+' ioPure(unit())\n} }')
 m.code.append('extern func allocateScheme() IO[runtime.Scheme] = "github.com/reoring/gofx/ports/client-go/kubernetes/scheme/host".NewScheme')
 m.code.append('func newRegistry() IO[Registry] { do {\n value <- allocateScheme()\n addMeta(value)\n addToScheme(value)\n codec <- codecs(value)\n parameter <- parameters(value)\n serializer <- withoutConversion(codec)\n decoder <- decoder(codec)\n ioPure(Registry(value,codec,parameter,serializer,decoder))\n} }')
 m.code.append('func generatedSerializer(registry Registry,features features.FeatureRegistry) IO[runtime.NegotiatedSerializer] { do {\n factory <- rest.codecFactoryForGeneratedClient(registry.scheme,registry.codecs,features)\n withoutConversion(factory)\n} }')
 m.code.append('func newClientRegistry(features features.FeatureRegistry) IO[Registry] { do {\n registry <- newRegistry()\n serializer <- generatedSerializer(registry,features)\n ioPure(registry{negotiatedSerializer:serializer})\n} }')
 save('kubernetes/scheme/register.gt',m.render());save('kubernetes/scheme/host/registration.go',host)
 print('Generated',len(order),'ordered external API scheme registrations')
if __name__=='__main__':generate()
