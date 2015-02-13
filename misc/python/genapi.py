#!/usr/bin/env python3
'''
Copyright (2015) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.

Devin Cook <devcook@sandia.gov>

Minimega binding generator for Python

This tool can take a json string from stdin that describes the minimega cli,
and produce a python API that will talk to the minimega process over the Unix
domain socket. In other words, run it like so:

    minimega -cli | ./genapi.py > minimega.py

The resulting minimega.py file will provide easy access to the minimega
commands from python.
'''


import jinja2


TEMPLATE = 'template.py'
CMD_TYPES = {
    'optionalItem': 1 << 0,
    'literalItem':  1 << 1,
    'commandItem':  1 << 2,
    'stringItem':   1 << 3,
    'choiceItem':   1 << 4,
    'listItem':     1 << 5,
}


def buildCommand(context, subs, cmd):
    ''' recursively build command tree
    context := dict describing the cmd context
    subs    := list of subcommands
    cmd     := json command object
    '''

    #cmdName needs to be a valid Python identifier
    cmdName = subs[0]
    cmdName = ''.join(filter(str.isalpha, cmdName))
    if cmdName in context:
        if 'subcommands' not in context[cmdName]:
            context[cmdName]['subcommands'] = {}
        buildCommand(context[cmdName]['subcommands'], subs[1:], cmd)
    elif len(subs) > 1:
        #still have more recursing to do
        context[cmdName] = {'subcommands': {}}
        buildCommand(context[cmdName]['subcommands'], subs[1:], cmd)
    else:
        #leaf node
        context[cmdName] = cmd


def render(cmds):
    context = {
                'version':  '2.0.dev0',
                'cmds':     {},
              }
    for c in cmds:
        cmd = c['shared_prefix'].split()
        buildCommand(context['cmds'], cmd, c)

    with open(TEMPLATE, 'rb') as tfile:
        t = jinja2.Template(tfile.read().decode())
    return t.render(context)


if __name__ == '__main__':
    import sys
    import json

    cmds = json.loads(sys.stdin.read())

    sys.stdout.write(render(cmds))

