(function() {
  const template = `
    <div
      :class="{ reserved: isReserved, available: !isReserved, up: isUp, down: !isUp, active: isSelected }"
      class="list-group-item list-group-item-action node unselected"
      draggable="true"
      style="opacity: 1; width:100%; padding: 12px; padding-left: 0px; padding-right: 0px; cursor: pointer;"
      tabindex="-1"
      v-on:click.stop="selectNode()"
    >{{ nodeID }}</div>
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
