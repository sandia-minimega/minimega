/*
Phenix apps customize experiments using metadata.

Default Apps

  * ntp.go:     configures a NTP server into the experiment infrastructure
  * serial.go:  configures a Serial interface on a VM image
  * startup.go: configures minimega startup injections based on OS type
  * user.go:    used to shell out with JSON payload to custom user apps
  * vyatta.go:  used to customize a vyatta router image, including setting
                interfaces, ACL rules, IPSec VPN settings, etc

Custom User Apps

Custom user apps are interacted with through STDIN and STDOUT. The phenix
`user.go` app will pass a single argument to the custom user app for the
current configuration stage, and JSON blob to the custom user app through
STDIN. This JSON blob will contain the experiment data based on the latest
version of the `Experiment` schema. The phenix will block further actions and
wait for STDOUT return of an updated JSON blob from the custom user app. In
addition to the JSON blob, the custom user app should exit with a value of 0.
If the custom user app is returning log(s) or any error messages, those
should be written to STDERR (and in the case of an error, the exit value
should be non-0). Custom user apps must 1) be in the user's PATH, 2) be
executable, and 3) follow the naming convention `phenix-app-<name>`.

On the command line, the user app should expect the experiment stage to be
passed as the one and only argument: configure, pre-start, post-start, or
cleanup.

On STDIN, the user app should expect the JSON form of the `types.Experiment`
struct to be passed.

ON STDOUT, the user app should return the JSON form of the experiment,
whether or not it was modified. For `configure` and `pre-start` stages, only
modifications to the experiment spec are saved. For `post-start` and
`cleanup` stages, only modifications to the `apps` experiment status key
(which is expected to be a JSON object) are saved. It's best practice for
each user app to add a top-level key to the `apps` JSON object with the
name of the user app as the key and any metadata in a JSON object as the
value.

Example Custom User App

  import json, sys

  def eprint(*args):
    print(*args, file=sys.stderr)

  def main() :
    if len(sys.argv) != 2:
      eprint("must pass exactly one argument on the command line")
      sys.exit(1)

    exp = json.loads(sys.stdin.read())

    # This user app only cares about the configure stage.
    if sys.argv[1] != 'configure':
      print(json.dumps(exp['spec']))
      sys.exit(0)

    for n in exp['spec']['topology']['nodes']:
      for d in n['hardware']['drives']:
        d['image'] = 'm$.qc2'

    print(json.dumps(exp))
*/
package app
