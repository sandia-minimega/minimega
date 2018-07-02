import {store} from './store/index.js';
import {VlanListGroup} from './components/VLANListGroup.js';
import {VlanCentricGraph} from './components/VLANCentricGraph.js';

const app = new Vue({
    el: '#app',

    store: store,

    components: {
        VlanListGroup,
        VlanCentricGraph,
    },

    mounted: function () {
        this.$store.dispatch('getAllVMs');
        setInterval(() => this.$store.dispatch('getAllVMs'), 5000);
    },

    methods: {
        vlanNodeClicked(vlanName) {
            console.log(vlanName);
            this.$refs["list"].show(vlanName);
        },
    }
});
