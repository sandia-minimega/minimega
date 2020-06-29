/*
Phenix apps customize experiments using metadata.
Phenix apps allow for the customization of experiments using metadata present

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

Example Custom User App

  import json, sys

  def eprint(*args):
    print(*args, file=sys.stderr)

  def main() :
    if len(sys.argv) != 2:
      eprint("must pass exactly one argument on the command line")
      sys.exit(1)

    # This user app only cares about the configure stage.
    if sys.argv[1] != 'configure':
      print(json.dumps(spec))
      sys.exit(0)

    spec = json.loads(sys.stdin.read())

    for n in spec['topology']['nodes']:
      for d in n['hardware']['drives']:
        d['image'] = 'm$.qc2'

    print(json.dumps(spec))
*/
package app
