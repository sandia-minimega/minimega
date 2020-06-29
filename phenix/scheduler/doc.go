/*
Phenix schedulers customize VM scheduling on cluster nodes using VM hardware
requirements, network configurations, etc. and cluster node capacity.

Default Schedulers

  * isolate-experiment.go: isolates all experiment VMs on a single cluster node
  * round-robin.go:        assigns experiment VMs to cluster nodes in a
                           round-robin fashion
  * subnet-compute.go:     assigns experiment VMs to cluster nodes based on
                           interface VLAN assignments

Custom User Schedulers

Custom user schedulers are interacted with through STDIN and STDOUT. The
phenix `user.go` scheduler will pass a JSON blob to the custom user scheduler
through STDIN. This JSON blob will contain the experiment data based on the
latest version of the `Experiment` schema. The phenix will block further
actions and wait for STDOUT return of an updated JSON blob from the custom
user scheduler. In addition to the JSON blob, the custom user scheduler
should exit with a value of 0. If the custom user scheduler is returning
log(s) or any error messages, those should be written to STDERR (and in the
case of an error, the exit value should be non-0). Custom user schedulers
must 1) be in the user's PATH, 2) be executable, and 3) follow the naming
convention `phenix-scheduler-<name>`.

Example Custom User Scheduler

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
*/
package scheduler
