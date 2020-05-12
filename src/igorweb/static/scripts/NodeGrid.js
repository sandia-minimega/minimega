/*
 * NodeGrid.js
 *
 * The NodeGrid component component displays reservation information
 * as a grid. Nodes are color coded to indicate their power (on/off)
 * status and reservation availability.
 *
 */
(function() {
  const template = `
    <div class="col">
      <div
        class="card mx-auto text-center"
        id="nodegridcard"
        style="background-color: #e7ccff; border: none;"
      >
        <div
          class="card-body"
          id="nodegrid"
          v-on:click.stop=""
        >
          <table id="node-grid">
            <tr v-for="r in rows">
              <template v-for="c in columns">
                <node
                  v-bind:id="getNodeInfo(c, r)['NodeID']"
                  v-bind:node-info="getNodeInfo(c, r)"
                  @nodeClicked="nodeClickedListener"
                  @nodeShiftClicked="nodeShiftClickedListener"
                  @nodeCtrlClicked="nodeCtrlClickedListener"
                ></node>
              </template>
            </tr>
          </table>
        </div>
      </div>
    </div>
  `;

  window.NodeGrid = {
    template: template,

    components: {
      Node,
    },

    mounted() {

      $('.node').on('mousedown', (event) => {
        this.selection.start = event.target['id'];

        $('.node').on('mouseover', (event) => {
          this.selection.end = event.target['id'];
          let min = parseInt(this.selection.start, 10);
          let max = parseInt(this.selection.end, 10);
          if (min > max) {
            min = parseInt(this.selection.end, 10);
            max = parseInt(this.selection.start, 10);
          }

          const nodes = [];
          for (let i = min; i <= max; i++) {
            nodes.push(i);
          }

          this.$store.dispatch('selectNodes', nodes);
        });

        return false;
      });

      $(window).on('mouseup', (event) => {
        $('.node').off('mouseover');
        this.selection.start = null;
        this.selection.end = null;
      });
    },

    data() {
      return {
        selection: {start: null, end: null},
        lastClickedNode: null
      };
    },

    methods: {
      getNodeInfo(column, row) {
        const start = this.$store.getters.startNode;
        const width = this.$store.getters.rackWidth;
        const index = start + (row*width + column%width);
        return this.$store.getters.nodes[index];
      },

      numCols() {
        return this.$store.getters.rackWidth;
      },

      numRows() {
        return Math.ceil(this.$store.getters.nodeCount/this.$store.getters.rackWidth);
      },

      nodeClickedListener(clickedNode) {
        this.lastClickedNode = clickedNode;
      },

      nodeShiftClickedListener(clickedNode) {
        var selectedNodes = this.$store.state.selectedNodes;

        // create an array with all numbers betwen clickedNodeID and lastClickedNodeID
        let minNodeID = Math.min(clickedNode.nodeID, this.lastClickedNode.nodeID);
        let maxNodeID = Math.max(clickedNode.nodeID, this.lastClickedNode.nodeID);
        let nodesInRange = Array.from(Array(maxNodeID - minNodeID + 1), (_, i) => i + minNodeID);

        if (selectedNodes.includes(clickedNode.nodeID)) {
          // unselect the nodes in range if the node being clicked is already selected
          selectedNodes = selectedNodes.filter(nodeID => !nodesInRange.includes(nodeID));
        } else {
          // select the nodes in range if the node being clicked is not already selected
          selectedNodes = selectedNodes.concat(nodesInRange);
        }

        this.$store.dispatch('selectNodes', selectedNodes);
      },

      nodeCtrlClickedListener(clickedNode) {
        var selectedNodes = this.$store.state.selectedNodes;

        // check if node is already selected
        if (selectedNodes.includes(clickedNode.nodeID)) {
          selectedNodes = selectedNodes.filter(val => clickedNode.nodeID != val);
        } else {
          selectedNodes.push(clickedNode.nodeID);
        }

        this.$store.dispatch('selectNodes', selectedNodes);
      }

    },

    computed: {
      columns() {
        const a = [];

        for (let i = 0; i < this.numCols(); i++) {
          a.push(i);
        }

        return a;
      },

      rows() {
        const a = [];

        for (let j=0; j < this.numRows(); j++) {
          a.push(j);
        }

        return a;
      },
    },
  };
})();
