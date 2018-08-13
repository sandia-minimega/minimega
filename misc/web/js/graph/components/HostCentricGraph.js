/* global _ */

import {MmCanvas} from './MmCanvas.js';
import {MmGraph} from './MmGraph.js';

// HostCentricGraph contains a MmGraph that draws a Host-Centric (?)
// representation of a network. Buttons are included to recenter the
// graph and expand/reduce the size of the nodes in the
// drawing.
//
//
// Examples:
//
//     <host-centric-graph></host-centric-graph>
//

const template = `
    <div>
        <h3>Host-Centric Graph</h3>
        <div class="btn-toolbar">
            <div class="btn-group">
                <button class="btn btn-default" v-on:click="recenter()">
                    <i class="fa fa-repeat"></i>
                </button>
            </div>
            <div class="btn-group pull-right">
                <button
                    class="btn btn-default"
                    v-on:click="nodeRadius < 15 ? nodeRadius++ : nodeRadius">
                        <i class="fa fa-expand"></i>
                </button>
                <button
                    class="btn btn-default"
                    v-on:click="nodeRadius > 3 ? nodeRadius-- : nodeRadius">
                        <i class="fa fa-compress"></i>
                </button>
            </div>
        </div>
        <mm-canvas>
            <mm-graph
             ref="graph"
             :nodes="nodes"
             :links="links"
             >
           </mm-graph>
        </mm-canvas>
    </div>
    `;

export const HostCentricGraph = {
  template: template,

  // Other components used by this Vue template
  components: {
    MmCanvas,
    MmGraph,
  },

  // Computed values are recomputed whenever dependencies change. If
  // dependencies don't change, the cached return value is used.
  computed: {
    // Returns an Array of nodes to be drawn. That is, an array of
    // Objects representing Hosts is returned with relevant style
    // information. See the template and MmGraph for more details.
    nodes() {
      // Returns the color of the VM based on the number of
      // VLANs it's connected to
      const fillColor = (vm) => {
        switch (vm.vlan.length) {
        case 0:
          // Unconnected VM
          return 'red';
        case 1:
          // Regular VM
          return 'blue';
        default:
          // "Router"
          return 'green';
        }
      };


      return _.map(this.$store.state.vms, (vm) => {
        return {
          id: vm.id,
          radius: this.nodeRadius,
          fillStyle: fillColor(vm),
        };
      });
    },

    // Returns an Array of links to be drawn. That is, an array of
    // Objects representing links between Hosts is returned
    links() {
      // Create an Object that maps VLAN name to an Array of VMs in that VLAN
      const machines = _.groupBy(
        this.$store.getters.connectedMachines,
        (vm) => vm.vlan[0]
      );

      // Create an Array of link Objects. For each router, create a
      // link to every VM in every VLAN that the router is connected
      // to.
      const connections = _.map(this.$store.getters.routers, (router) => {
        return _.map(router.vlan, (vlan) => {
          return _.map(machines[vlan], (m) => {
            return {
              source: router.id,
              target: m.id,
            };
          });
        });
      });

      return _.flatten(connections);
    },
  },

  // Local data for HostCentricGraph
  data() {
    return {
      // The radius of the nodes
      nodeRadius: 5,
    };
  },

  // Convenience methods
  methods: {
    // Recenters and reheats the graph
    recenter() {
      this.$refs['graph'].recenter();
    },
  },
};
