import {MmCanvas} from './MmCanvas.js';
import {MmGraph} from './MmGraph.js';


const template = `
    <div>
        <h3>VLAN-Centric Graph</h3>
        <div class="btn-toolbar">
            <div class="btn-group">
                <button class="btn btn-default" v-on:click="recenter()">
                    <i class="fa fa-repeat"></i>
                </button>
            </div>
            <div class="btn-group pull-right">
                <button class="btn btn-default" v-on:click="nodeRadius < 15 ? nodeRadius++ : nodeRadius">
                    <i class="fa fa-expand"></i>
                </button>
                <button class="btn btn-default" v-on:click="nodeRadius > 3 ? nodeRadius-- : nodeRadius">
                    <i class="fa fa-compress"></i>
                </button>
            </div>
        </div>
        <mm-canvas>
            <mm-graph
             ref="graph"
             :nodes="nodes"
             :node-style="nodeStyle"
             :links="links"
             v-on:node-click="nodeClicked($event)"
             >
           </mm-graph>
        </mm-canvas>
    </div>
    `;

export var VlanCentricGraph = {
    template: template,

    components: {
        MmCanvas,
        MmGraph
    },

    computed: {
        nodes() {
            return _.map(this.$store.getters.vlans, (vlan) => {
                return { id: vlan.name };
            });
        },

        nodeStyle() {
            return _.chain(this.$store.getters.vlans)
                .map((vlan) => {
                    const key = vlan.name;
                    const value = {
                        radius: this.nodeRadius,
                        fill: vlan.name == this.selectedVlan ? "blue" : "red",
                    };

                    return [key, value];
                })
                .object()
                .value();
        },

        links() {
            // TODO links!
            return [];
        },
    },

    data() {
        return {
            nodeRadius: 5,
            selectedVlan: null,
        };
    },

    methods: {
        recenter() {
            this.$refs["graph"].recenter();
        },

        nodeClicked(nodeId) {
            // nodeId is null if clicked away from node
            this.selectedVlan = nodeId;

            if (nodeId) {
                this.$emit("vlan-selected", nodeId);
            }
        },
    }
};
