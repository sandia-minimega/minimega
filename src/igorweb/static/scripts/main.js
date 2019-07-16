const app = new Vue({
  // Main element
  el: '#app',

  // Vuex storage
  store: store,

  // Components used in #app
  components: {
    Alert,
    ReservationInfo,
    NewReservationModal,
    EditReservationModal,
    DeleteReservationModal,
    ExtendReservationModal,
    PowerModal,
  },

  computed: {
    alertMessage() {
      return this.$store.state.alert;
    },

    selectedReservation() {
      return this.$store.state.selectedReservation;
    },
  },

  // Runs after the Vue component (the whole app, in this case) has
  // been mounted and is ready-to-go
  mounted: function() {
    // Load recently used images from Local Storage
    let imgs = JSON.parse(localStorage.getItem('usrImages'));
    if (!imgs) {
      imgs = [];
    }
    this.$store.commit('setRecentImages', imgs);


    // Load initial reservation data
    this.$store.commit('updateReservations', INITIALRESERVATIONS);

    // Fetch reservation data
    this.$store.dispatch('getReservations');

    // Set an interval, so that we fetch more reservation data every 5 seconds
    setInterval(() => this.$store.dispatch('getReservations'), 5000);
  },

  // Helper methods
  methods: {
    handleReservationAction(action, resName) {
      switch (action) {
        case 'edit':
          this.$refs['editResModal'].show(resName);
          break;
      }
    },

    showNewResForm() {
      this.$refs['newResModal'].show();
    },

    showActionBar() {
      $(this.$refs['actionbar']).show();
      $(this.$refs['actionbar']).addClass('active');
    },

    showDeleteModal() {
      this.$refs['deleteModal'].show();
    },

    showPowerModal() {
      this.$refs['powerModal'].show();
    },

    showExtendModal() {
      this.$refs['extendModal'].show();
    },

    clearSelection() {
      this.$store.dispatch('clearSelection');
    },
  },
});
