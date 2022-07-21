(function() {
  const template = `
    <tr
      :class="{active: selected}"
      class="res clickable"
      v-on:click.stop="selectReservation(reservation)"
    >
      <td>{{ reservation.Name }}</td>
      <td v-if="columns.includes('Owner')">{{ reservation.Owner }}</td>
      <td v-if="columns.includes('Group')">{{ reservation.Group }}</td>
      <td v-if="columns.includes('Start Time')">{{ reservation.Start }}</td>
      <td v-if="columns.includes('End Time')">{{ reservation.End }}</td>
      <td v-if="columns.includes('Nodes')">{{ nodeCount }}</td>
      <td v-if="columns.includes('Range')">{{ reservation.Range }}</td>
      <td>
        <template
          v-if="reservation.CanEdit"
        >
          <button
            class="btn btn-sm btn-primary"
            title="Edit Reservation"
            v-on:click="console.log('boonk'); action('edit')"
          >
            <i class="oi oi-pencil"></i>
          </button>
          <button
            class="btn btn-sm btn-success"
            title="Extend Reservation"
            v-on:click="action('extend')"
          >
            <i class="oi oi-clock"></i>
          </button>
          <button
            class="btn btn-sm btn-warning"
            title="Change Power State"
            v-on:click="action('power')"
          >
            <i class="oi oi-power-standby"></i>
          </button>
          <button
            class="btn btn-sm btn-danger"
            title="Delete Reservation"
            v-on:click="action('delete')"
          >
            <i class="oi oi-x"></i>
          </button>
        </template>
      </td>
    </tr>
  `;

  window.ReservationTableRow = {
    template: template,

    props: {
      reservation: {
        type: Object,
      },
      columns: {
        type: Array,
      },
    },

    computed: {
      nodeCount() {
        return this.reservation.Nodes.length;
      },

      selected() {
        if (this.$store.state.selectedReservation != null) {
          if (this.$store.state.selectedReservation.Name == this.reservation.Name) {
            return true;
          }
        }

        return this.$store.state.selectedNodes.some((n) => {
          return this.reservation.Nodes.includes(n);
        });
      },
    },

    methods: {
      action(kind) {
        this.selectReservation(this.reservation);
        this.$emit('res-action', kind, this.reservation.Name);
      },

      selectReservation(r) {
        this.$store.dispatch('selectReservation', r);
      },
    },
  };
})();
