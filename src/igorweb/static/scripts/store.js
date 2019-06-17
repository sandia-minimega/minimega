Vue.use(Vuex);

window.store = new Vuex.Store({
  strict: true,

  state: {
    selectedReservation: null,
    selectedNodes: [],
    reservations: [],
    alert: '',
  },

  // Convenience methods for accessing application-wide data.  The
  // return values are cached until dependencies change
  getters: {
    reservations: (state) => {
      return state.reservations;
    },

    selectedRange: (state, getters) => {
      if (state.selectedNodes.length == 0) {
        return '';
      }
      return `${getters.clusterPrefix}[${toRange(state.selectedNodes)}]`;
    },

    nodes: (state, getters) => {
      const n = {};

      for (let i = STARTNODE; i <= ENDNODE; i++) {
        n[i] = {
          NodeID: i,
          Waiting: true,
        };
      }

      if (state.reservations.length < 1) {
        return n;
      }

      for (let i = STARTNODE; i <= ENDNODE; i++) {
        n[i] = {
          NodeID: i,
          Up: true,
          Reservation: null,
          Waiting: false,
        };
      }

      // The first reservation is our list of down nodes
      const down = state.reservations[0].Nodes;
      for (let i = 0; i < down.length; i++) {
        n[down[i]].Up = false;
      }

      for (let i = 1; i < state.reservations.length; i++) {
        const r = state.reservations[i];

        for (let j = 0; j < r.Nodes.length; j++) {
          const nodeID = r.Nodes[j];

          n[nodeID].Reservation = r;
          n[nodeID].Reservation.Range = `${getters.clusterPrefix}[${toRange(r.Nodes)}]`;
        }
      }

      return n;
    },

    clusterPrefix: () => {
      return CLUSTER;
    },

    nodeCount: () => {
      return ENDNODE-STARTNODE+1;
    },

    startNode: () => {
      return STARTNODE;
    },

    rackWidth: () => {
      return RACKWIDTH;
    },
  },

  // Methods to change applicaiton-wide state. Note that this is not
  // the place to perform asyncronous actions.
  mutations: {
    updateReservations(state, rs) {
      state.reservations = rs;
    },

    setAlert(state, msg) {
      state.alert = msg;
    },

    setSelectedNodes(state, nodes) {
      state.selectedNodes = nodes;
    },

    setSelectedReservation(state, res) {
      state.selectedReservation = res;
    },
  },

  // Methods to perform actions that eventaually result in a state
  // mutation. This is a good place to perform any necessary
  // asynchronous operations (like grabbing JSON from igorweb
  // server's API).
  actions: {

    // Fetch new "vm info" from /vms/info.json and update
    // state.vms accordingly.
    getReservations({commit}) {
      $.get(
          'run/',
          {run: 'igor show'},
          function(data) {
            const response = JSON.parse(data);
            commit('updateReservations', response.Extra);
          }
      );
    },

    selectReservation({commit}, r) {
      commit('setSelectedNodes', r.Nodes);
      commit('setSelectedReservation', r);
    },

    selectNodes({commit}, n) {
      commit('setSelectedNodes', n);
      commit('setSelectedReservation', null);
    },

    clearSelection({commit}) {
      commit('setSelectedNodes', []);
      commit('setSelectedReservation', null);
    },
  },


});

function toRange(nodes) {
  if (nodes.length == 0) {
    return '[]';
  }

  let result = '';

  let min = null;
  for (let i = 0; i < nodes.length-1; i++) {
    const n = nodes[i];
    const m = nodes[i+1];

    if (min === null) {
      min = n;
    }

    if (m === n+1) {
      continue;
    }

    if (result !== '') {
      result += ', ';
    }

    if (min === n) {
      result += `${min}`;
    } else {
      result += `${min}-${n}`;
    }

    min = null;
  }

  const n = nodes[nodes.length-1];
  if (result !== '') {
    result += ', ';
  }

  if (min === n || min === null) {
    result += `${n}`;
  } else {
    result += `${min}-${n}`;
  }

  return result;
}
