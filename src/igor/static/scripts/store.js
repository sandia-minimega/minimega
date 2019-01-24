Vue.use(Vuex);

window.store = new Vuex.Store({
  strict: true,

  state: {
    selectedReservation: null,
    selectedNode: null,
    reservations: [],
    alert: "",
  },

  // Convenience methods for accessing application-wide data.  The
  // return values are cached until dependencies change
  getters: {
    reservations: (state) => {
      return state.reservations;
    },

    nodes: (state) => {
      let n = {};

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
      let down = state.reservations[0].Nodes;
      for (let i = 0; i < down.length; i++) {
        n[down[i]].Up = false;
      }

      for (let i = 1; i < state.reservations.length; i++) {
        let r = state.reservations[i];

        for (let j = 0; j < r.Nodes.length; j++) {
          let nodeID = r.Nodes[j];

          n[nodeID].Reservation = r;
          n[nodeID].Reservation.Range = toRange(r.Nodes);
        }
      }

      return n;
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

    setSelectedNode(state, id) {
      state.selectedNode = id;
    },

    setSelectedReservation(state, name) {
      state.selectedReservation = name;
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
          let response = JSON.parse(data);
          commit('updateReservations', response.Extra);
        }
      );
    },

    selectReservation({commit}, r) {
      commit('setSelectedNode', null);
      commit('setSelectedReservation', r);
    },

    selectNode({commit}, n) {
      commit('setSelectedNode', n);
      commit('setSelectedReservation', null);
    },

    clearSelection({commit}) {
      commit('setSelectedNode', null);
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
    let n = nodes[i];
    let m = nodes[i+1];

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

  let n = nodes[nodes.length-1];
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
