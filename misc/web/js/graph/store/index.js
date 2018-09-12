/* global Vue, Vuex, d3, _, s */
Vue.use(Vuex);


// Vuex manages the application-wide state, so that state must be
// modified in a predictable way.
window.store = new Vuex.Store({
  // Application-wide state
  state: {
    // An Array of VM info. Contains data loaded from /vms/info.json
    vms: [],
  },

  // Convenience methods for accessing application-wide data.  The
  // return values are cached until dependencies change (e.g., if
  // state.vms is updated).
  getters: {

    // An Object, where each property is a VLAN name (string), and
    // each value is an Object with the following properties:
    //
    //     name:   The name of the VLAN (string)
    //     number: The parsed VLAN number (number)
    //     vms:    Array of VMs connected to that VLAN (Array of Objects)
    vlans: (state) => {
      return _.chain(state.vms)
        .map( (vm) => vm.vlan )
        .flatten()
        .uniq() // Array of unique VLAN names
        .map( (vlan) => [
          vlan,
          {name: vlan,
            number: parseVlanNumber(vlan),
            vms: _.filter(state.vms, (vm) => vm.vlan.includes(vlan))},
        ])
        .object() // Object mapping VLAN names to name/number/vms
        .value();
    },

    // An Array containing all VMs that are connected to exactly one VLAN
    connectedMachines: (state) => {
      return _.filter(state.vms, (x) => x.vlan.length == 1);
    },

    // An Array containing VMs connected to exactly zero VLANs
    unconnectedMachines: (state) => {
      return _.filter(state.vms, (x) => x.vlan.length == 0);
    },

    // An Array containing VMs connected to two or more VLANs
    routers: (state) => {
      return _.filter(state.vms, (x) => x.vlan.length > 1);
    },
  },

  // Methods to change applicaiton-wide state. Note that this is not
  // the place to perform asyncronous actions.
  mutations: {
    // Update state.vms
    setVMs(state, vms) {
      // Keep the VMs sorted by "id".
      state.vms = _.sortBy(vms, (vm) => vm.id);
    },
  },

  // Methods to perform actions that eventaually result in a state
  // mutation. This is a good place to perform any necessary
  // asynchronous operations (like grabbing JSON from miniweb
  // server's API).
  actions: {

    // Fetch new "vm info" from /vms/info.json and update
    // state.vms accordingly.
    getAllVMs({commit}) {
      let path = window.location.pathname;
      path = path.substr(0, path.indexOf('/graph'));

      // Grab the new vm info, and clean it up before committing.
      d3.json(path+'/vms/info.json')
        .then( (vminfo) => commit('setVMs', cleanUp(vminfo)) );
    },
  },


});

function parseVlanNumber(name) {
  let match;
  name = s.trim(name);

  // If the name is only digits, that's the VLAN number.
  match = /^(\d+)$/.exec(name);
  if (match) {
    return Number(match[1]);
  }

  // Otherwise we expect the VLAN number in parentheses
  match = /\((\d+)\)$/.exec(name);
  if (match) {
    return Number(match[1]);
  }

  // Otherwise the name is junk.
  console.log(`Unable to parse VLAN Number from "${name}"`);
  return -1;
}

// Clean up objects loaded from /vms/info.json
function cleanUp(vminfo) {
  for (let vm of vminfo) {
    // Parse the tags as JSON
    vm.tags = JSON.parse(vm.tags);

    // Convert the "list" of vlans from a string to an Array
    vm.vlan = s.trim(vm.vlan, '[]').split(', ');
  }

  return vminfo;
}
