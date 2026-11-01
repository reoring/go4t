#!/usr/bin/env python3
"""Regenerate the descriptor before its consumers, without compiling product code.
Copyright The Kubernetes Authors. Licensed under Apache-2.0.
"""
import argparse,os,pathlib,subprocess,sys
HERE=pathlib.Path(__file__).resolve().parent
if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('--output-root',type=pathlib.Path,help='Write an isolated generated tree; continue reading descriptors and bindings from the source repository.')
    args=parser.parse_args()
    environment={**os.environ,'PYTHONDONTWRITEBYTECODE':'1'}
    scripts=('resources.py','generate.py','scheme.py','fakes.py','clientset.py','coverage.py')
    if args.output_root is not None:
        output_root=args.output_root.resolve()
        if output_root==HERE.parents[2]:
            parser.error('--output-root must differ from the source repository')
        environment['CLIENTGO_TYPED_OUTPUT_ROOT']=str(output_root)
        scripts=scripts[1:]
    for script in scripts:
        subprocess.run([sys.executable,str(HERE/script)],check=True,env=environment)
