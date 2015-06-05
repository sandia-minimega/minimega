#!/usr/bin/env python
'''
Copyright (2015) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.

Devin Cook <devcook@sandia.gov>

Minimega binding generator for Python

This tool can take a json string from stdin that describes the minimega cli,
and produce a python API that will talk to the minimega process over the Unix
domain socket. In other words, run it like so:

    ./genapi.py /path/to/minimega > minimega.py

The resulting minimega.py file will provide easy access to the minimega
commands from python.
'''


import os
import jinja2


TEMPLATE = os.path.join(os.path.dirname(os.path.realpath(__file__)),
                        'template.py')
CMD_TYPES = {
    'optionalItem': 1 << 0,
    'literalItem':  1 << 1,
    'commandItem':  1 << 2,
    'stringItem':   1 << 3,
    'choiceItem':   1 << 4,
    'listItem':     1 << 5,
}
CMD_BLACKLIST = [
    # filter these commands out so they don't show up in the API
    # this list should also include any commands you want to generate manually
    'help',
    #'mesh send',
]

# API Version:
VERSION = '2.0a1'
# minimega version (set this before rendering)
MM_VERSION = 'UNKNOWN'


def parseCmdType(type):
    ''' return a string describing the type of the argument
    type := bitmask 'type' from cmd json object
    '''

    #zero out the optionalItem bit
    type &= 0xfffffffe

    #this will only return one type, the first one encountered
    for name, value in CMD_TYPES.items():
        if type & value:
            return name

    raise ValueError('Unknown command type', type)


def parseArgs(args):
    ''' return a list containing the parsed argument info
    args := list of args from the json cmd object
    '''
    for arg in args:
        arg.update({
                    'optional': bool(arg['type'] & CMD_TYPES['optionalItem']),
                    'type':     parseCmdType(arg['type']),
                   })
    return args


def buildCommand(context, subs, cmd):
    ''' recursively build command tree
    context := dict describing the cmd context
    subs    := list of subcommands
    cmd     := json command object
    '''

    #cmdName needs to be a valid Python identifier
    cmdName = subs[0]
    cmdName = ''.join(filter(str.isalpha, str(cmdName)))

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
        prefix_len = len(cmd['shared_prefix'].split())
        cmd['candidates'] = list(map(parseArgs, [arg[prefix_len:] for arg in
                                                 cmd['parsed_patterns']]))


def render(cmds):
    context = {
                'version':    VERSION,
                'mm_version': MM_VERSION,
                'cmds':       {},
              }
    for c in cmds:
        cmd = c['shared_prefix']

        #make sure this isn't a blacklisted or cli-specific command
        if cmd.startswith('.') or cmd in CMD_BLACKLIST:
            continue

        buildCommand(context['cmds'], cmd.split(), c)

    with open(TEMPLATE, 'rb') as tfile:
        t = jinja2.Template(tfile.read().decode())
    return t.render(context)


if __name__ == '__main__':
    import sys
    import subprocess
    import json

    if len(sys.argv) != 2:
        print('Usage: {} /path/to/minimega'.format(sys.argv[0]))
        exit()

    mm_bin = sys.argv[1]
    cmds = json.loads(subprocess.check_output([mm_bin, '-cli']))
    version_str = subprocess.check_output([mm_bin, '--version'])
    MM_VERSION = version_str.split(None, 2)[1]

    sys.stdout.write(render(cmds))

