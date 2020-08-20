/*
 * store.js
 *
 * This script sets up the storage for the whole application.
 *
 * Vuex is a library that provides a way to define a global, mutable
 * storage that allows our application to stay reactive. That is, Vuex
 * provides a sane, well-defined way for us to keep track of our
 * application state while ensuring that all of our components stay
 * reactive to changes in application state.
 *
 * TL;DR it lets us use global variables without losing our minds
 *
 */
Vue.use(Vuex);

window.store = new Vuex.Store({
  strict: true,

  state: {
    // The currently selected reservation in the ReservationTable
    selectedReservation: null,

    // The currently selected nodes in the NodeGrid
    selectedNodes: [],

    // An array of reservation data we received from the last "igor show"
    reservations: [],

    // The currently displayed alert message
    alert: '',

    // An array of the default kernel/init images
    defaultImages: [],

    // An array of images recently used by the user
    recentImages: [],
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
    allImages(state) {
      return [].concat(state.recentImages, state.defaultImages);
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
      // sort in ascending order and remove duplicates
      nodes.sort((a, b) => a > b);
      nodes = nodes.filter((_, i) => nodes[i] != nodes[i-1]);

      state.selectedNodes = nodes;
    },

    setSelectedReservation(state, res) {
      state.selectedReservation = res;
    },
    addRecentImage(state, img) {
      // Add image to the beginning of the list of recent images
      state.recentImages.unshift(img);
    },
    setRecentImages(state, imgs) {
      state.recentImages = imgs;
    },
    setDefaultImages(state, imgs) {
      state.defaultImages = imgs;
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
    saveRecentImage({commit, getters, state}, kernelInitPair) {
      const kernelPath = kernelInitPair.kernelPath;
      const initrdPath = kernelInitPair.initrdPath;
      const tmp = kernelPath.split('/');
      const image = {
        name: tmp[tmp.length - 1].split('.')[0] + ' (recent)',
        kernel: kernelPath,
        initrd: initrdPath,
      };

      if (!getters.allImages.some((x) => x.name == image.name)) {
        commit('addRecentImage', image);
        localStorage.setItem('usrImages', JSON.stringify(state.recentImages));
      }
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
