(function() {
  const template = `
      <div
        draggable="true"
        tabindex="-1"
        style="opacity: 1; width:100%; padding: 12px; padding-left: 0px; padding-right: 0px; cursor: pointer;"
        class="list-group-item list-group-item-action node unselected"
        :class="{ reserved: isReserved, available: !isReserved, up: isUp, down: !isUp, active: reservationIsSelected }"
        v-on:click="selectNode()"
      >
        {{ nodeID }}
      </div>
    `;

  window.Node = {
    template: template,

    props: {
      nodeInfo: {
        type: Object,
      },
    },

    computed: {
      nodeID() {
        return this.nodeInfo.NodeID;
      },

      isWaiting() {
        return this.nodeInfo.Waiting;
      },

      isReserved() {
        if (this.nodeInfo.Reservation) {
          return this.nodeInfo.Reservation.Owner != '';
        }

        return false;
      },

      reservationIsSelected() {
        if (this.$store.state.selectedReservation == null || this.nodeInfo.Reservation == null) {
          return false;
        }
        return this.$store.state.selectedReservation.Name == this.nodeInfo.Reservation.Name;
      },

      isUp() {
        if (this.nodeInfo.Up) {
          return this.nodeInfo.Up;
        }

        return false;
      },
    },

    methods: {
      selectNode() {
        this.$store.dispatch('selectNode', this.nodeInfo);
      }
    },
  };
})();
