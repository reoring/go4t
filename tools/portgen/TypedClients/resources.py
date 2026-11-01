#!/usr/bin/env python3
"""Extract client-gen resources from pinned source, never from discovery.
Copyright 2026 The go4t Authors. SPDX-License-Identifier: Apache-2.0
"""
import argparse, hashlib, json, pathlib, re

ROOT = pathlib.Path(__file__).resolve().parents[3]
DEFAULT_SOURCE = ROOT.parent.parent / '.tmp/client-go-v0.37.2'
VERSION = 'v0.37.2'
REVISION = '72c52dd0cc3b9099b36af11290b0304045dd1ada'

def imports(text):
    return {alias or path.rsplit('/',1)[-1]:path for alias,path in re.findall(r'^\s*(?:(\w+)\s+)?"([^"]+)"',text,re.M)}

def qualify(value, aliases):
    value = value.strip().lstrip('*')
    return re.sub(r'\b(\w+)\.(\w+)', lambda m: aliases.get(m[1],m[1])+'.'+m[2], value)

def functions(text):
    pattern = r'^func\s+(?:\(([^\n]+)\)\s+)?(\w+)\s*(\([^\n]*)\{'
    for m in re.finditer(pattern,text,re.M):
        start=m.end(); depth=1; pos=start
        while depth and pos<len(text):
            depth += (text[pos]=='{')-(text[pos]=='}'); pos+=1
        yield {'receiver':m[1], 'name':m[2], 'signature':m[3].strip(), 'body':text[start:pos-1], 'line':text.count('\n',0,m.start())+1}

def call_argument(text,name):
    hit=re.search(r'\b'+name+r'\(',text)
    if not hit:return None
    start=hit.end();depth=1;pos=start
    while depth and pos<len(text):
        depth+=(text[pos]=='(')-(text[pos]==')');pos+=1
    return text[start:pos-1]

def extract(source):
    resources={}; all_sources=[]
    bindings=json.loads((ROOT/'ports/client-go/deps/coverage.json').read_text())['object_descriptors']
    api=pathlib.Path('/home/vagrant/go/pkg/mod/k8s.io/api@'+VERSION)
    for path in sorted((source/'kubernetes').rglob('*.go')):
        text=path.read_text(); rel=path.relative_to(source).as_posix()
        all_sources.append({'path':rel,'sha256':hashlib.sha256(text.encode()).hexdigest(),'generated':'Code generated' in text,'test':path.name.endswith('_test.go')})
        if path.parent.name=='fake' or path.name.endswith('_test.go'): continue
        hit=re.search(r'gentype\.NewClient(?:WithListAndApply|WithList|WithApply)?\[([^\]]+)\]\(\s*"([^"]+)"',text)
        if not hit: continue
        aliases=imports(text); types=[qualify(v,aliases) for v in hit[1].split(',')]
        object_type=types[0]; kind=object_type.rsplit('.',1)[1]; parts=path.parts
        group,version=parts[-3:-1]
        client=(path.parent/(group+'_client.go'))
        if not client.exists():
            clients=list(path.parent.glob('*_client.go')); assert len(clients)==1, path; client=clients[0]
        clienttext=client.read_text()
        resource={'group': '' if group=='core' else object_type.split('/')[2]+'.k8s.io', 'group_package':group,'version':version,'kind':kind,'resource':hit[2], 'scope':'Namespaced' if re.search(r'func new\w+\(c \*\w+, namespace string\)',text) else 'Cluster', 'object_type':object_type,'list_type':next((t for t in types[1:] if not 'ApplyConfiguration' in t),None),'apply_type':next((t for t in types[1:] if 'ApplyConfiguration' in t),None),'verbs':[], 'extensions':[], 'source':rel,'client_source':client.relative_to(source).as_posix(),'prefers_protobuf':'PrefersProtobuf' in text,'markers':[], 'symbols':[]}
        register=(api/group/version/'register.go').read_text()
        group_name=re.search(r'GroupName\s*=\s*"([^"]*)"',register)
        resource['group']=group_name[1] if group_name else re.search(r'Group:\s*"([^"]*)"',register)[1]
        api_types=(api/group/version/'types.go').read_text()
        declaration=re.search(r'((?://[^\n]*\n)+)type '+re.escape(kind)+r' struct',api_types)
        resource['markers']=re.findall(r'//\s*(\+genclient[^\n]*)',declaration[1]) if declaration else []
        for field in ('object','list'):
            fq=resource[field+'_type']
            if fq:
                b=bindings[fq]
                resource[field+'_binding']={**b,'type_name':fq.rsplit('.',1)[1],'constructor':'new'+fq.rsplit('.',1)[1]}
        resource['getter']=re.search(r'type (\w+)Getter interface',text)[1]
        resource['group_version']={'group':resource['group'],'version':version}
        interface=re.search(r'type '+kind+r'Interface interface \{(.*?)\n\}',text,re.S)
        if interface:
            resource['verbs']=re.findall(r'^\s*(\w+)\(',interface[1],re.M)
        resources[object_type]=resource
    for resource in resources.values():
        directory=source/pathlib.Path(resource['source']).parent
        for path in sorted(directory.glob('*.go')):
            if path.name.endswith('_test.go'): continue
            text=path.read_text(); aliases=imports(text)
            for fn in functions(text):
                receiver=fn['receiver'] or ''
                if not re.search(r'\*'+re.escape(resource['resource'])+r'\b',receiver,re.I): continue
                body=fn.pop('body'); fn.update(source=path.relative_to(source).as_posix(),generated='Code generated' in text)
                fn['input_result_signature']=qualify(fn['signature'],aliases)
                fn['http_verb']=next(iter(re.findall(r'\.(Get|Post|Put|Delete|Patch)\(',body)),None)
                fn['subresources']=re.findall(r'\bSubResource\("([^"]+)"\)',body)
                fn['name_expression']=call_argument(body,'Name')
                fn['namespace_expression']=call_argument(body,'Namespace')
                fn['body_expression']=call_argument(body,'Body')
                fn['body_source']=body.strip()
                resource['extensions'].append(fn)
        resource['symbols']=[resource['kind']+'Interface',resource['kind']+'Expansion']+resource['verbs']
        resource['typed_client']=True
        resource['fake_extensions']=[]
        for path in sorted((directory/'fake').glob('*.go')):
            if path.name.endswith('_test.go'):continue
            text=path.read_text();aliases=imports(text)
            for fn in functions(text):
                if not re.search(r'\*fake'+re.escape(resource['resource'])+r'\b',fn['receiver'] or '',re.I):continue
                body=fn.pop('body')
                fn.update(source=path.relative_to(source).as_posix(),generated='Code generated' in text,input_result_signature=qualify(fn['signature'],aliases),body_source=body.strip(),action_constructors=re.findall(r'\b(New\w+Action\w*)\(',body))
                resource['fake_extensions'].append(fn)
    for path in sorted((source/'listers').rglob('*.go')):
        text=path.read_text()
        hit=re.search(r'listers.New\[\*(\w+\.\w+)\]\(indexer,\s*\w+\.Resource\("([^"]+)"\)',text)
        if not hit: continue
        fq=qualify(hit[1],imports(text))
        if fq in resources:
            resources[fq]['lister_source']=path.relative_to(source).as_posix()
            continue
        group,version=path.parts[-3:-1];kind=fq.rsplit('.',1)[1]
        register=(api/group/version/'register.go').read_text()
        match=re.search(r'GroupName\s*=\s*"([^"]*)"',register)
        groupname=match[1] if match else re.search(r'Group:\s*"([^"]*)"',register)[1]
        b=bindings[fq]
        resources[fq]={'group':groupname,'group_package':group,'version':version,'kind':kind,'resource':hit[2],'scope':'Namespaced' if 'NamespaceLister interface' in text else 'Cluster','object_type':fq,'list_type':None,'apply_type':None,'object_binding':{**b,'type_name':kind,'constructor':'new'+kind},'source':path.relative_to(source).as_posix(),'lister_source':path.relative_to(source).as_posix(),'typed_client':False,'verbs':[],'extensions':[],'markers':[],'symbols':[],'prefers_protobuf':False}
    return {'schema_version':1,'source':{'version':VERSION,'revision':REVISION},'resources':resources,'sources':all_sources}

def main():
    p=argparse.ArgumentParser(); p.add_argument('--source',type=pathlib.Path,default=DEFAULT_SOURCE); p.add_argument('--output',type=pathlib.Path,default=ROOT/'tools/portgen/resources.json'); args=p.parse_args()
    result=extract(args.source); args.output.parent.mkdir(parents=True,exist_ok=True); args.output.write_text(json.dumps(result,indent=2,sort_keys=True)+'\n')
    print(f"Extracted {len(result['resources'])} resources and {len(result['sources'])} source files into {args.output}")
if __name__=='__main__': main()
