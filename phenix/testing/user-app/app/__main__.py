import json, sys


def eprint(*args):
    print(*args, file=sys.stderr)


def main() :
    if len(sys.argv) != 2:
        eprint("must pass exactly one argument on the command line")
        sys.exit(1)

    exp = json.loads(sys.stdin.read())

    for n in exp['spec']['topology']['nodes']:
        for d in n['hardware']['drives']:
            d['image'] = 'm$.qc2'

    print(json.dumps(exp))