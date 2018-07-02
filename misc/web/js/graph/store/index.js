Vue.use(Vuex);

export const store = new Vuex.Store({
    state: {
        vms: [],
    },

    getters: {
        vlans: state => {
            return  _.chain(state.vms)
                .map( vm => vm.vlan )
                .flatten()
                .uniq()
                .map( vlan => [
                    vlan,
                    { name: vlan,
                      number: Number(/\((\d+)\)/.exec(vlan)[1]),
                      vms: _.filter(state.vms, vm => vm.vlan.includes(vlan)) }
                ])
                .object()
                .value();
        },

        connectedMachines: state => {
            return _.filter(state.vms, (x) => x.vlan.length == 1);
        },

        unconnectedMachines: state => {
            return _.filter(state.vms, (x) => x.vlan.length == 0);
        },

        routers: state => {
            return _.filter(state.vms, (x) => x.vlan.length > 1);
        },
    },

    actions: {
        getAllVMs ({ commit }) {
            var path = window.location.pathname;
            path = path.substr(0, path.indexOf("/graph"));
            d3.json(path+"/vms/info.json")
                .then((vminfo) => commit('setVMs', cleanUp(vminfo)));
        },
    },

    mutations: {
        setVMs (state, vms) {
            state.vms = vms;
        },
    }
});

function cleanUp(vminfo) {
    for (let vm of vminfo) {
        vm.tags = JSON.parse(vm.tags);
        vm.vlan = s.trim(vm.vlan, '[]').split(", ")
    }

    return vminfo;
}
