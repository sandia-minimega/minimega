(function() {
  const template = `
    <td
      :class="{ reserved: isReserved, available: !isReserved, up: isUp, down: !isUp, active: isSelected }"
      class="node unselected"
      draggable="true"
      tabindex="-1"
      v-on:click.stop="selectNode($event)"
      v-bind:title="title"
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
      selectNode(evt) {
        let selectedNodes = this.$store.state.selectedNodes;

        if (evt.shiftKey && selectedNodes.length > 0) {
          this.$emit('nodeShiftClicked', this);
        } else if (evt.ctrlKey && selectedNodes.length > 0) {
          this.$emit('nodeCtrlClicked', this);
        } else {
          this.$store.dispatch('selectNodes', [this.nodeInfo.NodeID]);
        }

        this.$emit('nodeClicked', this);
      },

    },

  };
})();
