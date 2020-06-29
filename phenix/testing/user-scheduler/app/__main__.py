import json, sys


def eprint(*args):
    print(*args, file=sys.stderr)


def main() :
    if len(sys.argv) != 1:
        eprint("no arguments expected on the command line")
        sys.exit(1)

    spec = json.loads(sys.stdin.read())
    sched = spec.schedules

    for n in spec['topology']['nodes']:
        for h in n['general']['hostname']:
            sched[h] = 'compute0'

    print(json.dumps(spec))