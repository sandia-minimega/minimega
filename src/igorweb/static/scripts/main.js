'use strict';

var app = new Vue({
  // Main element
  el: '#app',
  // Vuex storage
  store: store,
  // Components used in #app
  components: {
    Alert: Alert,
    ReservationInfo: ReservationInfo,
    NewReservationModal: NewReservationModal,
    EditReservationModal: EditReservationModal,
    DeleteReservationModal: DeleteReservationModal,
    ExtendReservationModal: ExtendReservationModal,
    PowerModal: PowerModal,
  },
  computed: {
    alertMessage: function alertMessage() {
      return this.$store.state.alert;
    },
    selectedReservation: function selectedReservation() {
      return this.$store.state.selectedReservation;
    },
  },
  // Runs after the Vue component (the whole app, in this case) has
  // been mounted and is ready-to-go
  mounted: function mounted() {
    var _this = this;

    var imgs = JSON.parse(localStorage.getItem('usrImages'));

    if (!imgs) {
      imgs = [];
    }

    this.$store.commit('setRecentImages', imgs); // Load initial reservation data

    this.$store.commit('updateReservations', INITIALRESERVATIONS); // Fetch reservation data

    this.$store.dispatch('getReservations'); // Set an interval, so that we fetch more reservation data every 5 seconds

    setInterval(function() {
      return _this.$store.dispatch('getReservations');
    }, 5000);
  },
  // Helper methods
  methods: {
    handleReservationAction: function handleReservationAction(action, resName) {
      switch (action) {
        case 'edit':
          this.$refs['editResModal'].show(resName);
          break;
      }
    },
    showNewResForm: function showNewResForm() {
      this.$refs['newResModal'].show();
    },
    showActionBar: function showActionBar() {
      $(this.$refs['actionbar']).show();
      $(this.$refs['actionbar']).addClass('active');
    },
    showDeleteModal: function showDeleteModal() {
      this.$refs['deleteModal'].show();
    },
    showPowerModal: function showPowerModal() {
      this.$refs['powerModal'].show();
    },
    showExtendModal: function showExtendModal() {
      this.$refs['extendModal'].show();
    },
    clearSelection: function clearSelection() {
      this.$store.dispatch('clearSelection');
    },
  },
});
