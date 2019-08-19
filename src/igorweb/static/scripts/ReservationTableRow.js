(function() {
  const template = `
    <tr
      :class="{active: selected}"
      class="res clickable"
      v-on:click.stop="selectReservation(reservation)"
    >
      <td>{{ reservation.Name }}</td>
      <td>{{ reservation.Owner }}</td>
      <td>{{ reservation.Group }}</td>
      <td class="current">{{ reservation.Start }}</td>
      <td>{{ reservation.End }}</td>
      <td>{{ nodeCount }}</td>
      <td>{{ reservation.Range }}</td>
      <td>
        <template
          v-if="reservation.CanEdit"
        >
          <button
            class="btn btn-primary"
            v-on:click="$emit('res-action', 'edit', reservation.Name)"
          >
            <i class="oi oi-pencil"></i>
          </button>
          <button
            class="btn btn-success"
            v-on:click="$emit('res-action', 'extend', reservation.Name)"
          >
            <i class="oi oi-clock"></i>
          </button>
          <button
            class="btn btn-warning"
            v-on:click="$emit('res-action', 'power', reservation.Name)"
          >
            <i class="oi oi-power-standby"></i>
          </button>
          <button
            class="btn btn-danger"
            v-on:click="$emit('res-action', 'delete', reservation.Name)"
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
      selectReservation(r) {
        this.$store.dispatch('selectReservation', r);
      },
    },
  };
})();
