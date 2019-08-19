(function() {
  const template = `
    <td
      :class="{ reserved: isReserved, available: !isReserved, up: isUp, down: !isUp, active: isSelected }"
      class="node unselected"
      draggable="true"
      tabindex="-1"
      v-bind:title="title"
      v-on:click.stop="selectNode()"
    >{{ nodeID }}</td>
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

      title() {
        let resInfo = 'Available';
        if (this.nodeInfo.Reservation) {
          resInfo = `Name: ${this.nodeInfo.Reservation.Name}\nOwner: ${this.nodeInfo.Reservation.Owner}`;
        }

        let power = this.isUp ? "Powered On" : "Powered Off";
        let reserved = this.isReserved ? "" : "Available";

        return `${resInfo}\n${power}`;
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

      isSelected() {
        return this.$store.state.selectedNodes.includes(this.nodeInfo.NodeID);
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
        this.$store.dispatch('selectNodes', [this.nodeInfo.NodeID]);
      },
    },
  };
})();
