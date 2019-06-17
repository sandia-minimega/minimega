(function() {
  const template = `
    <tr
      class="res clickable mdl"
      :class="{active: selected}"
      v-on:click.stop="selectReservation(reservation)"
    >
      <td class="mdl">{{ reservation.Name }}</td>
      <td class="mdl">{{ reservation.Owner }}</td>
      <td class="mdl current">{{ reservation.Start }}</td>
      <td class="mdl">{{ reservation.End }}</td>
      <td class="mdl">{{ nodeCount }}</td>
      <td class="mdl">{{ reservation.Range }}</td>
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
        if (this.$store.state.selectedReservation == null) {
          return false;
        }
        return this.$store.state.selectedReservation.Name == this.reservation.Name;
      },
    },

    methods: {
      selectReservation(r) {
        this.$store.dispatch('selectReservation', r);
      },
    },
  };
})();
