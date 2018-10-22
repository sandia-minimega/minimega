/* global _ */

(function() {
  // VLANCentricGraph contains a MmGraph that draws a VLAN-Centric
  // representation of a network. Buttons are included to recenter the
  // graph and expand/reduce the size of the nodes in the
  // drawing. Clicking a VLAN will cause its color to change and emit a
  // vlan-selected event.
  //
  // Events:
  //
  //   - vlan-selected: Fires whenever a node (VLAN) is clicked. The
  //                    VLAN name is included with the event
  //
  // Examples:
  //
  //     <vlan-centric-graph
  //         v-on:vlan-selected="doSomething($event)">
  //     </vlan-centric-graph>
  //

  const template = `
      <div>
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
               v-on:node-click="nodeClicked($event)"
               >
             </mm-graph>
          </mm-canvas>
      </div>
      `;

  window.VlanCentricGraph = {
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
      // Objects representing VLANs is returned with relevant style
      // information. See the template and MmGraph for more details.
      nodes() {
        return _.map(this.$store.getters.vlans, (vlan) => {
          return {
            id: vlan.name,
            radius: this.nodeRadius,
            fillStyle: vlan.name == this.selectedVlan ? 'blue' : 'red',
          };
        });
      },

      // Returns an Array of links to be drawn. That is, an array of
      // Objects representing links between VLANs is returned. If
      // one or more VMs is connected to two VLANs, a link is drawn
      // between those two VLANs.
      links() {
        return _.map(this.$store.getters.routers, (router) => {
          if (router.vlan.length > 2) {
            console.log('Found vm connected to >2 vlans');
          }
          return {
            source: router.vlan[0],
            target: router.vlan[1],
            strokeStyle: '#000',
          };
        });
      },
    },

    // Local data for VlanCentricGraph
    data() {
      return {
        // The radius of the nodes
        nodeRadius: 5,

        // The selected VLAN, if any
        selectedVlan: null,
      };
    },

    // Convenience methods
    methods: {

      // Recenters and reheats the graph
      recenter() {
        this.$refs['graph'].recenter();
      },

      // Runs when a node is clicked
      nodeClicked(nodeId) {
        // nodeId is null if clicked away from node
        this.selectedVlan = nodeId;

        // If the node ID is non-null
        if (nodeId) {
          this.$emit('vlan-selected', nodeId);
        }
      },
    },
  };
})();
