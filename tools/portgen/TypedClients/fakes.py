#!/usr/bin/env python3
"""Generate native fake typed capabilities over the native action engine."""
import json,pathlib,re
from generate import Module,common,ordinary,extension,low,q,save,DATA,BIND
from resources import DEFAULT_SOURCE,imports,qualify,functions

def gvr(r):return 'schema.groupVersionResourceWithResource(schema.groupVersionResourceWithVersion(schema.groupVersionResourceWithGroup(schema.newGroupVersionResource(),'+q(r['group'])+'),'+q(r['version'])+'),'+q(r['resource'])+')'
def gvk(r):return 'schema.groupVersionKindWithKind(schema.groupVersionKindWithVersion(schema.groupVersionKindWithGroup(schema.newGroupVersionKind(),'+q(r['group'])+'),'+q(r['version'])+'),'+q(r['kind'])+')'

def fake_extension(m,r,e,shape):
 name,aa,rt,_=shape;T=m.typ(r['object_type']);namespace='namespace' if r['scope']=='Namespaced' else '""';method=e['name'].removesuffix('WithContext')
 fakefile=DEFAULT_SOURCE/pathlib.Path(e['source']).parent/'fake'
 # A handwritten override may live beside the generated resource file.
 sources=[]
 for path in sorted(fakefile.glob('*.go')):
  if path.name.endswith('_test.go'):continue
  for fn in functions(path.read_text()):
   if fn['name']==method and fn['receiver'] and re.search(r'\*fake'+re.escape(r['resource'])+r'\b',fn['receiver'],re.I):sources.append((fn,path))
 if not sources and method.endswith('WithContext'):return None
 fbody=sources[0][0]['body'] if sources else ''
 def toobj(n):
  t=next(t for key,t in aa if key==n);return 'objects.toObject('+descriptor_type(t)+','+n+')'
 def descriptor_type(t):
  if t.startswith('Option['):t=t[7:-1]
  alias,kind=t.split('.');path=m.imports[alias].removeprefix('deps/');return m.descriptor(path+'.'+kind)
 def newtype(t):
  alias,kind=t.split('.');path=m.imports[alias].removeprefix('deps/');return m.constructor(path+'.'+kind)
 if name=='proxyGet':return name,aa,rt,'testing.invokesProxy(fake,testing.newProxyGetAction(resource,'+namespace+',scheme,name,port,path,params))'
 if name=='getLogs':
  m.imp('rest/fake','fakerest')
  core=m.imp('deps/k8s.io/api/core/v1','core')
  return name,aa,rt,'''do {
 let action = (testing.newAction(testing.Generic,"get",resource,namespace)){subresource:"log",value:Some(testing.packPayload(testing.runtimeObjectPayloadKey(),core.podLogOptionsToRuntimeObject(opts)))}
 let defaultObject = runtime.unknownWithRaw(runtime.newUnknown(),Some("fake logs"))
 result <- testing.invokes(fake,action,Some(runtime.unknownToRuntimeObject(defaultObject)))
 let decoded = objects.fromObject(runtime.unknownObjectType(),result.value)
 let bytes = match decoded { None => "fake logs"; Some(value) => match runtime.unknownGetRaw(value) { None => ""; Some(value) => value } }
 let error = match result.error { Some(error) => Some(error); None => match decoded { Some(_) => None; None => Some(errors.errorNew("fake Pods.GetLogs expected reactor to return *runtime.Unknown, got "+nativefake.objectTypeName(result.value))) } }
 registry <- scheme.newRegistry()
 client <- fakerest.newClient(registry.negotiatedSerializer,schema.groupVersionWithVersion(schema.newGroupVersion(),"v1"),"/api/v1/namespaces/"+namespace+"/pods/"+name+"/log",environment.clock)
 request <- fakerest.request(client)
 fakerest.requestWithResponseBytes(request,bytes,error)
}'''.replace('core.',core+'.')
 if name=='getFieldSelector':
  return name,aa,rt,'do { testing.invokes(fake,testing.newAction(testing.Generic,"get-field-selector",resource,""),None); native.fieldSelector([]) }'
 if name=='search':
  empty=newtype(rt);return name,aa,rt,'do { action <- testing.newListActionWithOptions(resource,kind,if namespace == "" { "default" } else { namespace },meta.newListOptions()); nativefake.invoke(fake,action,'+descriptor_type(rt)+','+empty+') }'
 if 'WithEventNamespace' in method:
  op=method.split('With')[0].lower();object=aa[0][0];alias,ty=aa[0][1].split('.');bodyNamespace='meta.objectMetaGetNamespace('+alias+'.'+low(ty)+'GetObjectMeta('+object+'))'
  ns='if namespace == "" { '+bodyNamespace+' } else { namespace }' if r['group_package']=='core' else namespace
  if op=='patch':action='testing.newPatchSubresourceActionWithOptions(resource,'+ns+',meta.objectMetaGetName('+alias+'.'+low(ty)+'GetObjectMeta('+object+')),"application/strategic-merge-patch+json",data,meta.newPatchOptions(),[])'
  else:action='testing.new'+op.title()+'ActionWithOptions(resource,'+ns+','+toobj(object)+',meta.new'+op.title()+'Options())'
  return name,aa,rt,'nativefake.invokeOptional(fake,'+action+','+descriptor_type(rt)+',Some('+object+'))'
 if name.startswith('apply'):
  conf=next((n,t) for n,t in aa if t.startswith('Option['));t=conf[1][7:-1];a,ty=t.split('.');codec=a+'.'+low(ty.removesuffix('ApplyConfiguration'))+'Marshal()'
  err=re.search(r'fmt.Errorf\("([^"]+)"\)',fbody)[1] if 'fmt.Errorf' in fbody else re.search(r'fmt.Errorf\("([^"]+)"\)',e['body_source'])[1]
  ret=rt[7:-1];parent=aa[0][0]
  return name,aa,rt,'nativefake.applyNamed(fake,resource,'+namespace+','+parent+','+q(e['subresources'][0])+','+conf[0]+','+codec+',opts,'+descriptor_type(ret)+','+newtype(ret)+','+q(err)+')'
 if name=='patchStatus':return name,aa,rt,'nativefake.invoke(fake,testing.newPatchSubresourceActionWithOptions(resource,"",nodeName,"application/strategic-merge-patch+json",data,meta.newPatchOptions(),["status"]),'+descriptor_type(rt)+','+newtype(rt)+')'
 # create-only handwritten actions ignore options exactly as upstream.
 if name in ('bind','evict','evictV1','evictV1beta1','rollback','finalize'):
  obj=aa[0][0];objcode=toobj(obj);ns=namespace
  if name=='bind':alias,ty=aa[0][1].split('.');ns='meta.objectMetaGetNamespace('+alias+'.'+low(ty)+'GetObjectMeta('+obj+'))'
  if name in ('rollback','finalize'):ns='""'
  sub={'bind':'binding','evict':'eviction','evictV1':'eviction','evictV1beta1':'eviction','rollback':'rollback','finalize':'finalize'}[name]
  res=gvr({**r,'group':'','version':'v1','resource':'pods'}) if r['kind']=='Eviction' else 'resource'
  action='(testing.newCreateActionWithOptions('+res+','+ns+','+objcode+',meta.newCreateOptions())){subresource:'+q(sub)+'}'
  if rt=='Unit':return name,aa,rt,'nativefake.execute(fake,'+action+',Some('+objcode+'))'
  return name,aa,rt,'nativefake.invoke(fake,'+action+','+descriptor_type(rt)+','+obj+')'
 # Generated get/update/create subresource methods independently preserve result type.
 sub=q(e['subresources'][0]);options=next((n for n,t in aa if t.endswith('Options')),'')
 if name.startswith('get'):
  action='testing.newGetSubresourceActionWithOptions(resource,'+namespace+','+sub+','+aa[0][0]+','+(options or 'meta.newGetOptions()')+')'
 elif name.startswith('update'):
  obj=next(n for n,t in aa if '.' in t and not t.endswith('Options'))
  # Handwritten beta approval ignores passed options.
  opt='meta.newUpdateOptions()' if 'NewRootUpdateSubresourceAction(' in fbody else options
  action='testing.newUpdateSubresourceActionWithOptions(resource,'+sub+','+namespace+','+toobj(obj)+','+opt+')'
 elif name.startswith('create'):
  obj=next(n for n,t in aa if '.' in t and not t.endswith('Options'));action='testing.newCreateSubresourceActionWithOptions(resource,'+aa[0][0]+','+sub+','+namespace+','+toobj(obj)+','+options+')'
 else:raise ValueError(('unhandled fake method',r['object_type'],name))
 return name,aa,rt,'nativefake.invoke(fake,'+action+','+descriptor_type(rt)+','+newtype(rt)+')'

def generate_group(rs):
 r0=rs[0];group=r0['group_package'];version=r0['version'];m=Module('fake');common(m)
 real=m.imp('kubernetes/typed/'+group+'/'+version,'real');m.imp('testing','testing');m.imp('kubernetes/fake/support','nativefake');m.imp('deps/k8s.io/apimachinery/pkg/runtime/schema','schema')
 grouptext=(DEFAULT_SOURCE/r0['client_source']).read_text();GT=re.search(r'type (\w+)Interface interface',grouptext)[1]+'Interface';getters=[]
 for r in rs:
  k=low(r['kind']);ns='namespace' if r['scope']=='Namespaced' else '""';methods=[]
  for name,aa,rt,code in ordinary(m,r):
   code=re.sub(r'gentype\.(\w+)',lambda v:'gentype.fake'+v[1][0].upper()+v[1][1:],code)
   code=code.replace(k+'ListAdapter()','real.'+k+'ListAdapter()').replace(k+'ApplyAdapter()','real.'+k+'ApplyAdapter()').replace(',registry,',',')
   if name=='deleteCollection':code=code.replace('client,options','client,real.'+k+'ListAdapter(),options')
   methods.append((name,aa,rt,code))
  for e in r['extensions']:
   shape=extension(m,r,e)
   if shape:methods.append(fake_extension(m,r,e,shape))
  byname={v[0]:v for v in methods}
  for e in r['extensions']:
   if e['name'].endswith('WithContext'):
    base=low(e['name'].removesuffix('WithContext'))
    if base in byname:methods.append((low(e['name']),*byname[base][1:]))
  values=[]
  for name,aa,rt,code in methods:values.append(name+':func('+','.join(n for n,t in aa)+') => { '+code+' }')
  extra=',namespace String' if r['scope']=='Namespaced' else ''
  ctor='new'+r['getter'].removesuffix('Getter')
  m.code.append('func '+ctor+'(environment rest.Environment,fake testing.Fake'+extra+') real.'+r['kind']+'Interface {\n let resource = '+gvr(r)+'\n let kind = '+gvk(r)+'\n let client = gentype.newFakeClient(fake,'+ns+',resource,kind,real.'+k+'ObjectAdapter())\n real.'+r['kind']+'Interface{'+',\n'.join(values)+'}\n}')
  getter=low(r['getter'].removesuffix('Getter'));getters.append(getter+':func('+('namespace' if extra else '')+') => '+ctor+'(environment,fake'+(',namespace' if extra else '')+')')
  if r['kind']=='Pod' and group=='core':
   core=m.imp('deps/k8s.io/api/core/v1','core')
   m.code.append(('func getBinding(fake testing.Fake,namespace String,name String) IO[outcome.Outcome[core.Binding]] { nativefake.invoke(fake,testing.newGetSubresourceActionWithOptions('+gvr(r)+',namespace,"binding",name,meta.newGetOptions()),core.bindingObjectType(),core.newBinding()) }').replace('core.',core+'.'))
 m.code.append('func newClient(environment rest.Environment,fake testing.Fake) real.'+GT+' { real.'+GT+' { restClient:None,'+','.join(getters)+' } }')
 save(pathlib.Path('kubernetes/typed')/group/version/'fake/clients.gt',m.render())

def main():
 groups={}
 for r in DATA['resources'].values():
  if r['typed_client']:groups.setdefault((r['group_package'],r['version']),[]).append(r)
 for rs in groups.values():generate_group(rs)
 print('Generated native fake families for',len(groups),'group versions')
if __name__=='__main__':main()
