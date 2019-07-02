'use strict';

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
    reservations: function reservations(state) {
      return state.reservations;
    },
    selectedRange: function selectedRange(state, getters) {
      if (state.selectedNodes.length == 0) {
        return '';
      }

      return ''.concat(getters.clusterPrefix, '[').concat(toRange(state.selectedNodes), ']');
    },
    nodes: function nodes(state, getters) {
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

      for (let _i = STARTNODE; _i <= ENDNODE; _i++) {
        n[_i] = {
          NodeID: _i,
          Up: true,
          Reservation: null,
          Waiting: false,
        };
      } // The first reservation is our list of down nodes


      const down = state.reservations[0].Nodes;

      for (let _i2 = 0; _i2 < down.length; _i2++) {
        n[down[_i2]].Up = false;
      }

      for (let _i3 = 1; _i3 < state.reservations.length; _i3++) {
        const r = state.reservations[_i3];

        for (let j = 0; j < r.Nodes.length; j++) {
          const nodeID = r.Nodes[j];
          n[nodeID].Reservation = r;
          n[nodeID].Reservation.Range = ''.concat(getters.clusterPrefix, '[').concat(toRange(r.Nodes), ']');
        }
      }

      return n;
    },
    clusterPrefix: function clusterPrefix() {
      return CLUSTER;
    },
    nodeCount: function nodeCount() {
      return ENDNODE - STARTNODE + 1;
    },
    startNode: function startNode() {
      return STARTNODE;
    },
    rackWidth: function rackWidth() {
      return RACKWIDTH;
    },
  },
  // Methods to change applicaiton-wide state. Note that this is not
  // the place to perform asyncronous actions.
  mutations: {
    updateReservations: function updateReservations(state, rs) {
      state.reservations = rs;
    },
    setAlert: function setAlert(state, msg) {
      state.alert = msg;
    },
    setSelectedNodes: function setSelectedNodes(state, nodes) {
      state.selectedNodes = nodes;
    },
    setSelectedReservation: function setSelectedReservation(state, res) {
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
    getReservations: function getReservations(_ref) {
      const commit = _ref.commit;
      $.get('run/', {
        run: 'igor show',
      }, function(data) {
        const response = JSON.parse(data);
        commit('updateReservations', response.Extra);
      });
    },
    selectReservation: function selectReservation(_ref2, r) {
      const commit = _ref2.commit;
      commit('setSelectedNodes', r.Nodes);
      commit('setSelectedReservation', r);
    },
    selectNodes: function selectNodes(_ref3, n) {
      const commit = _ref3.commit;
      commit('setSelectedNodes', n);
      commit('setSelectedReservation', null);
    },
    clearSelection: function clearSelection(_ref4) {
      const commit = _ref4.commit;
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

  for (let i = 0; i < nodes.length - 1; i++) {
    const _n = nodes[i];
    const m = nodes[i + 1];

    if (min === null) {
      min = _n;
    }

    if (m === _n + 1) {
      continue;
    }

    if (result !== '') {
      result += ', ';
    }

    if (min === _n) {
      result += ''.concat(min);
    } else {
      result += ''.concat(min, '-').concat(_n);
    }

    min = null;
  }

  const n = nodes[nodes.length - 1];

  if (result !== '') {
    result += ', ';
  }

  if (min === n || min === null) {
    result += ''.concat(n);
  } else {
    result += ''.concat(min, '-').concat(n);
  }

  return result;
}
