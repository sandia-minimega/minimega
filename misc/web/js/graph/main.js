import {store} from './store/index.js';
import {VlanListGroup} from './components/VLANListGroup.js';
import {HostCentricGraph} from './components/HostCentricGraph.js';
import {VlanCentricGraph} from './components/VLANCentricGraph.js';
import {DandelionGraph} from './components/DandelionGraph.js';

const app = new Vue({
  // Main element
  el: '#app',

  // Vuex storage
  store: store,

  // Components used in #app
  components: {
    VlanListGroup,
    HostCentricGraph,
    VlanCentricGraph,
    DandelionGraph,
  },

  // Top-level data items
  data() {
    return {
      // The desired graph view. VLAN-centric, Host-centric, or
      // Dandelion... centric.
      selectedView: 'Dandelion',
    };
  },

  // Runs after the Vue component (the whole app, in this case) has
  // been mounted and is ready-to-go
  mounted: function() {
    // Fetch VM data
    this.$store.dispatch('getAllVMs');

    // Set an interval, so that we fetch more VM data every 5 seconds
    // TODO: This should be configurable.
    setInterval(() => this.$store.dispatch('getAllVMs'), 5000);
  },

  // Helper methods
  methods: {
    // Runs whenever a VLAN node is clicked.
    vlanNodeClicked(vlanName) {
      // If the VLAN's drawer in the VlanListGroup is hidden,
      // then show it.
      this.$refs['list'].show(vlanName);
    },
  },
});
