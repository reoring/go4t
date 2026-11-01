#!/usr/bin/env python3
"""Generate an executable native family consumer, not a descriptor coverage test.

Each case calls the current public clientset and records the RoundTripper input
and decoded typed output. No product compilation or execution occurs here.
Copyright The Kubernetes Authors. Licensed under Apache-2.0.
"""
import json,pathlib
from collections import Counter
from generate import Module,ROOT,low,q,save


def main():
    data=json.loads((ROOT/'tools/portgen/resources.json').read_text())
    resources=[(key,r) for key,r in sorted(data['resources'].items()) if r.get('typed_client')]
    groups={(g,v):name for g,v,name in json.loads((ROOT/'tools/portgen/TypedClients/groups.json').read_text())}
    module=Module('family')
    module.imp('kubernetes/regression/family/support','fixture')
    module.imp('deps/k8s.io/apimachinery/pkg/apis/meta/v1','meta')
    cases=[]
    for index,(key,r) in enumerate(resources):
        operation=next((verb for verb in ('Get','List','Create') if verb in r['verbs']),None)
        if operation is None:
            if any(e['name']=='Evict' for e in r['extensions']):operation='Evict'
            else:raise ValueError(('resource lacks supported executable operation',key))
        api_version=r['group']+'/'+r['version'] if r['group'] else r['version']
        namespace='client-namespace' if r['scope']=='Namespaced' else ''
        getter=low(r['getter'])
        accessor=low(groups[r['group_package'],r['version']])
        client='harness.client.'+accessor+'().'+getter+'('+q(namespace)+')' if r['scope']=='Namespaced' else 'harness.client.'+accessor+'().'+getter+'()'
        binding=r['object_binding'];dep=module.imp(binding['module'].removeprefix('github.com/reoring/gofx/ports/client-go/'))
        response={'apiVersion':api_version,'kind':r['kind'],'metadata':{'name':'response-name','namespace':'response-namespace','resourceVersion':'response-rv'}}
        input_object={'apiVersion':api_version,'kind':r['kind'],'metadata':{'name':'input-name','namespace':'input-namespace'}}
        options={}
        setup=[]
        descriptor=dep+'.'+binding['descriptor']+'()'
        result_type=key
        result_kind=r['kind']
        if operation=='Get':
            options={'resourceVersion':'fixture-rv'}
            call=client+'.get("request-name",meta.getOptionsWithResourceVersion(meta.newGetOptions(),"fixture-rv"))'
        elif operation=='List':
            options={'resourceVersion':'fixture-rv'}
            call=client+'.list(meta.listOptionsWithResourceVersion(meta.newListOptions(),"fixture-rv"))'
            response={'apiVersion':api_version,'kind':r['kind']+'List','metadata':{'resourceVersion':'response-rv'},'items':[]}
            result_kind=r['kind']+'List';result_type=r['list_type']
            b=r['list_binding'];ldep=module.imp(b['module'].removeprefix('github.com/reoring/gofx/ports/client-go/'));descriptor=ldep+'.'+b['descriptor']+'()'
        else:
            type_meta='meta.typeMetaWithAPIVersion(meta.typeMetaWithKind(meta.newTypeMeta(),'+q(r['kind'])+'),'+q(api_version)+')'
            object_meta='meta.objectMetaWithNamespace(meta.objectMetaWithName(meta.newObjectMeta(),"input-name"),"input-namespace")'
            name=low(r['kind'])
            setup=['let object = '+dep+'.'+name+'WithTypeMeta('+dep+'.'+name+'WithObjectMeta('+dep+'.'+binding['constructor']+'(),'+object_meta+'),'+type_meta+')']
            if operation=='Create':
                options={'fieldManager':'family-fixture','dryRun':['All'],'fieldValidation':'Strict'}
                option_expr='meta.createOptionsWithFieldValidation(meta.createOptionsWithDryRun(meta.createOptionsWithFieldManager(meta.newCreateOptions(),"family-fixture"),Some(["All"])),"Strict")'
                call=client+'.create(object,'+option_expr+')'
            else:
                call=client+'.evict(object)'
                response={'apiVersion':'v1','kind':'Status','status':'Success','code':200}
        function='runCase'+str(index).zfill(3)
        code=['// '+r['source'], 'func '+function+'(harness fixture.Harness) IO[Bool] { do {',' fixture.begin(harness,'+q(json.dumps(response,separators=(',',':')))+')']
        code += [' '+line for line in setup]
        code.append(' result <- '+call)
        if operation=='Evict':code.append(' fixture.recordUnit(harness,'+q(key)+','+q(operation)+',result)')
        else:code.append(' fixture.record(harness,'+q(key)+','+q(operation)+','+q(result_type)+','+q(result_kind)+','+descriptor+',result)')
        code.append('} }');module.code.append('\n'.join(code))
        cases.append({'resource':key,'operation':operation,'source':r['source'],'client_source':r['client_source'],'group_accessor':accessor,'resource_getter':getter,'namespace':namespace,'request_name':'request-name' if operation=='Get' else None,'input':input_object if operation in ('Create','Evict') else None,'options':options,'response':response,'function':function,'scope':r['scope']})
    code=['func main() IO[Unit] { do {',' harness <- fixture.newHarness()']
    code += [' result'+str(i)+' <- '+c['function']+'(harness)' for i,c in enumerate(cases)]
    code += [' if '+ ' && '.join('result'+str(i) for i in range(len(cases)))+' { ioPure(unit()) } else { ioFail("native typed family driver observed request, decode, or cast failures; inspect emitted rows") }','} }']
    module.code.append('\n'.join(code))
    save('kubernetes/regression/family/main.gfx',module.render())
    manifest={'schema_version':1,'source':data['source'],'case_count':len(cases),'group_count':len({(r['group_package'],r['version']) for _,r in resources}),'selection':'Get then List then Create, otherwise source-defined Evict','config':{'host':'http://family.invalid','content_type':'application/json','accept_content_types':'application/json','qps':-1},'cases':cases}
    save('kubernetes/regression/family-cases.json',json.dumps(manifest,indent=2,sort_keys=True)+'\n')
    print('Generated',len(cases),'executable native cases across',manifest['group_count'],'groups:',dict(Counter(c['operation'] for c in cases)))
if __name__=='__main__':main()
