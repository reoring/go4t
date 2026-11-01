#!/usr/bin/env python3
"""Native clientset assembly with shared HTTP identity and explicit registries."""
import json,re
from generate import Module,save,ROOT,DEFAULT_SOURCE,low,GROUPS_PATH

def generate():
 groups=json.loads(GROUPS_PATH.read_text())
 source=(DEFAULT_SOURCE/'kubernetes/clientset.go').read_text()
 # Exact upstream field/constructor ordering, not alphabetical discovery order.
 order=re.findall(r'cs\.(\w+), err = (\w+)\.NewForConfigAndClient',source)
 aliases={a:p for a,p in re.findall(r'^\s*(\w+)\s+"k8s.io/client-go/(kubernetes/typed/[^\"]+)"',source,re.M)}
 lookup={(g,v):n for g,v,n in groups}
 groupinfo=[]
 for field,alias in order:
  path=aliases[alias];g,v=path.split('/')[-2:];groupinfo.append((field,path,lookup[g,v]))
 m=Module('kubernetes')
 for a,p in [('rest','rest'),('scheme','kubernetes/scheme'),('discovery','discovery'),('outcome','support/outcome'),('errors','deps/support')]:m.imp(p,a)
 fields=['discovery:func() discovery.DiscoveryInterface','isWatchListSemanticsUnSupported:Bool'];ctors=['discovery:func() => discovery.newDiscoveryClient(client)','isWatchListSemanticsUnSupported:false']
 for field,path,name in groupinfo:
  a=m.imp(path,field);fields.append(field+':func() '+a+'.'+name+'Interface');ctors.append(field+':func() => '+a+'.newClient(environment,registry,client)')
 m.code.append('type Interface = Interface { '+','.join(fields)+' }\nalias Clientset = Interface')
 m.code.append('func newClientset(environment rest.Environment,registry scheme.Registry,client rest.Interface) Clientset { Interface { '+','.join(ctors)+' } }')
 body='''func newForConfigAndClient(environment rest.Environment,config rest.Config,httpClient rest.HTTPClient) IO[outcome.Outcome[Option[Clientset]]] { do {
 config <- match config.rateLimiter { Some(_) => ioPure(config); None => if config.qps > 0.0 {
  if config.burst <= 0 { ioPure(config) } else { do { limiter <- environment.newRateLimiter(config.qps,config.burst); ioPure(config{rateLimiter:Some(limiter)}) } }
 } else { ioPure(config) } }
 if config.qps > 0.0 && config.burst <= 0 && (match config.rateLimiter { None => true; Some(_) => false }) { ioPure(outcome.failure(None,errors.errorNew("burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0"))) }
 else { do {
 registry <- scheme.newClientRegistry(environment.features)
'''
 for field,path,name in groupinfo:
  body+=f' {field} <- {field}.newForConfigAndClientWithRegistry(environment,registry,config,httpClient)\n match {field}.value {{ None => ioPure(outcome.from(None,{field}.error)); Some({field}) => do {{\n'
 body+=' discoveryClient <- discovery.newDiscoveryClientForConfigAndClient(environment,config,httpClient)\n match discoveryClient.value { None => ioPure(outcome.from(None,discoveryClient.error)); Some(discoveryClient) => ioPure(outcome.success(Some(Interface{discovery:func() => discoveryClient,isWatchListSemanticsUnSupported:false,'+','.join(field+':func() => '+field for field,_,_ in groupinfo)+'}))) }\n'
 body+=' } }\n'*len(groupinfo)+' } }\n} }'
 m.code.append(body)
 m.code.append('''func newForConfig(environment rest.Environment,config rest.Config) IO[outcome.Outcome[Option[Clientset]]] { do {
 let config = rest.setKubernetesDefaults(config)
 http <- rest.httpClientFor(environment,config)
 match http.value { None => ioPure(outcome.from(None,http.error)); Some(client) => newForConfigAndClient(environment,config,client) }
} }''')
 m.code.append('''func newForConfigOrDie(environment rest.Environment,config rest.Config) IO[Clientset] { do {
 result <- newForConfig(environment,config)
 match result.value { Some(client) => ioPure(client); None => ioFail(match result.error { Some(error) => errors.errorMessage(error); None => "client construction returned no client" }) }
} }''')
 save('kubernetes/clientset.gfx',m.render())
 f=Module('fake')
 for a,p in [('kubernetes','kubernetes'),('rest','rest'),('scheme','kubernetes/scheme'),('discoveryfake','discovery/fake'),('testing','testing'),('runtime','deps/k8s.io/apimachinery/pkg/runtime'),('apply','applyconfigurations'),('sync','support/sync'),('errors','deps/support')]:f.imp(p,a)
 f.code.append('type Clientset = Clientset { client:kubernetes.Interface,fake:testing.Fake,tracker:Option[testing.ObjectTracker] }')
 ctor=['discovery:func() => discovery','isWatchListSemanticsUnSupported:true']
 for field,path,name in groupinfo:
  a=f.imp(path+'/fake',field);ctor.append(field+':func() => '+a+'.newClient(environment,fake)')
 f.code.append('''func assemble(environment rest.Environment,fake testing.Fake,tracker Option[testing.ObjectTracker]) IO[Clientset] { do {
 discovery <- discoveryfake.newFakeDiscovery(fake)
 ioPure(Clientset(kubernetes.Interface{'''+','.join(ctor)+'''},fake,tracker))
} }''')
 f.code.append('func newEmptyClientset(environment rest.Environment) IO[Clientset] { do { fake <- testing.newFake(); assemble(environment,fake,None) } }')
 f.code.append('''func initialize(environment rest.Environment,tracker testing.ObjectTracker,objects List[runtime.Object]) IO[Clientset] { do {
 addObjects(tracker,objects)
 fake <- testing.newFake()
 testing.addReactor(fake,"*","*",testing.objectReaction(tracker))
 testing.addWatchReactor(fake,"*",func(ref) => do { action <- sync.readRef(ref); result <- tracker.watch(action.resource,action.namespace,[action.listOptions]); ioPure(testing.WatchReaction(true,result)) })
 assemble(environment,fake,Some(tracker))
} }
func addObjects(tracker testing.ObjectTracker,objects List[runtime.Object]) IO[Unit] { match objects {
 Nil => ioPure(unit())
 Cons(object,remaining) => do { result <- tracker.add(object); match result.error { Some(error) => ioFail(errors.errorMessage(error)); None => addObjects(tracker,remaining) } }
} }
func newSimpleClientset(environment rest.Environment,objects List[runtime.Object]) IO[Clientset] { do {
 registry <- scheme.newRegistry()
 tracker <- testing.newObjectTracker(registry.scheme,registry.decoder)
 initialize(environment,tracker,objects)
} }
func newClientset(environment rest.Environment,objects List[runtime.Object]) IO[Clientset] { do {
 registry <- scheme.newRegistry()
 converter <- apply.newTypeConverter(registry.scheme)
 tracker <- testing.newFieldManagedObjectTracker(registry.scheme,registry.decoder,converter)
 initialize(environment,tracker,objects)
} }
func isWatchListSemanticsUnSupported(client Clientset) Bool { true }
''')
 save('kubernetes/fake/clientset.gfx',f.render());print('Generated real and fake clientsets with',len(groupinfo),'group accessors')
if __name__=='__main__':generate()
