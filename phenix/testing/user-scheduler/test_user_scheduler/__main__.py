import json, sys


def eprint(*args):
    print(*args, file=sys.stderr)


def main() :
    if len(sys.argv) != 1:
        eprint("no arguments expected on the command line")
        sys.exit(1)

    spec = json.loads(sys.stdin.read())

    for n in spec['topology']['nodes']:
        h = n['general']['hostname']
        spec['schedules'][h] = 'compute0'

    print(json.dumps(spec))