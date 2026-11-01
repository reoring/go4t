#!/usr/bin/env python3
"""Emit exact exported-symbol provenance from the owning inventory."""
import json,pathlib,re
from resources import ROOT,VERSION,REVISION
from generate import low,save,GROUPS_PATH

def generate():
 root=ROOT/'ports/client-go';data=json.loads((ROOT/'tools/portgen/resources.json').read_text());inv=json.loads((root/'upstream-inventory.json').read_text())
 groups=json.loads(GROUPS_PATH.read_text());groupnames={(g,v):n for g,v,n in groups}
 rs=[r for r in data['resources'].values() if r['typed_client']]
 entries=[];files=[]
 for f in inv['files']:
  path=f['path']
  if not path.startswith('kubernetes/'):continue
  files.append({'source':path,'sha256':f['sha256'],'role':f.get('role','metadata'),'generated':f.get('generated',False)})
  if f.get('role')!='source':continue
  directory=str(pathlib.PurePosixPath(path).parent);parts=directory.split('/');fake=parts[-1]=='fake';module=directory
  if module=='kubernetes/scheme':output='kubernetes/scheme/register.gt'
  elif module in ('kubernetes','kubernetes/fake'):output=module+'/clientset.gt'
  else:output=module+'/clients.gt'
  candidates=[r for r in rs if str(pathlib.PurePosixPath(r['source']).parent)==directory.removesuffix('/fake')]
  for d in f.get('declarations',[]):
   if not d['exported']:continue
   name=d['name'];equivalent=None;note=''
   if directory=='kubernetes/scheme':
    equivalent={'Scheme':'Registry.scheme','Codecs':'Registry.codecs','ParameterCodec':'Registry.parameterCodec','AddToScheme':'addToScheme'}[name]
   elif directory in ('kubernetes','kubernetes/fake'):
    equivalent={'Interface':'Interface','Clientset':'Clientset','New':'newClientset','NewForConfig':'newForConfig','NewForConfigAndClient':'newForConfigAndClient','NewForConfigOrDie':'newForConfigOrDie','NewSimpleClientset':'newSimpleClientset','NewClientset':'newClientset','Tracker':'Clientset.tracker','IsWatchListSemanticsUnSupported':'Clientset.client.isWatchListSemanticsUnSupported','Discovery':'Interface.discovery'}.get(name)
    if equivalent is None:equivalent='Interface.'+low(name)
   else:
    group,version=parts[2:4];gt=groupnames[group,version]+'Interface'
    if name=='EventSinkImpl':output='kubernetes/typed/core/v1/event_expansion.gt';equivalent='EventSinkImpl'
    elif name=='GetInvolvedObjectNameFieldLabel':output='kubernetes/typed/core/v1/event_expansion.gt';equivalent=low(name)
    elif d.get('receiver')=='*EventSinkImpl':output='kubernetes/typed/core/v1/event_expansion.gt';equivalent='eventSink'+name
    elif name=='RESTClient':equivalent=gt+'.restClient';note='Option preserves nil on fake clients'
    elif name in ('New','NewForConfig','NewForConfigAndClient','NewForConfigOrDie'):
     equivalent={'New':'newClient','NewForConfig':'newForConfig','NewForConfigAndClient':'newForConfigAndClient','NewForConfigOrDie':'newForConfigOrDie'}[name]
    elif name.startswith('Fake') or name==groupnames[group,version]+'Client' or name==gt:equivalent='newClient -> real.'+gt if fake else gt
    else:
     for r in candidates:
      if name==r['getter']+'Getter':equivalent=gt+'.'+low(r['getter']);note='Getter capability is a callback field';break
      if name in (r['kind']+'Interface',r['kind']+'Expansion'):equivalent=('real.' if fake else '')+r['kind']+'Interface';break
      if name==r['getter'].removesuffix('Getter'):equivalent=gt+'.'+low(name);break
      receiver=d.get('receiver','').removeprefix('*').removeprefix('fake')
      if receiver.lower()==r['resource'].lower():equivalent=('getBinding' if name=='GetBinding' else ('real.' if fake else '')+r['kind']+'Interface.'+low(name));break
    if equivalent is None:raise RuntimeError(('unmapped symbol',path,name,d.get('receiver')))
   entries.append({'source':path,'line':d['line'],'kind':d['kind'],'upstream':name,'receiver':d.get('receiver'),'native_file':output,'native':equivalent,'note':note})
 report={'schema_version':1,'source':{'version':VERSION,'revision':REVISION},'typed_resources':len(rs),'group_versions':len(groups),'files':files,'exports':entries,'verification':'not run; this is source provenance, not behavioral acceptance'}
 save('kubernetes/coverage.json',json.dumps(report,indent=2,sort_keys=True)+'\n')
 print('Mapped',len(entries),'exported source declarations across',len(files),'pinned files')
if __name__=='__main__':generate()
