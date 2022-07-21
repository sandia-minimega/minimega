/*
 * ReservationTable.js
 *
 * The ReservationTable component lists reservations in a table. Each
 * row in the table is a ReservationTableRow.
 *
 * Whenever a ReservationTableRow emits a "res-action" event, the
 * event and its payload are re-emitted.
 *
 * A ReservationTable has clickable column headers that allow the user
 * to sort reservations by Name, Owner, Group, etc.
 *
 */

(function() {
  const template = `
    <table class="table table-hover table-borderless">
      <thead>
        <tr>
          <!-- Reservation table headers with sorting arrows -->
          <th
            class="clickable"
            scope="col"
            v-on:click="changeSort('name')"
          >
            Name
            <span
              :class="{'oi-arrow-thick-top': !reversed, 'oi-arrow-thick-bottom': reversed}"
              class="oi"
              v-if="sortBy == 'name'"
            ></span>
          </th>
          <th
            class="clickable"
            scope="col"
            v-if="columns.includes('Owner')"
            v-on:click="changeSort('owner')"
          >
            Owner
            <span
              :class="{'oi-arrow-thick-top': !reversed, 'oi-arrow-thick-bottom': reversed}"
              class="oi"
              v-if="sortBy == 'owner'"
            ></span>
          </th>
          <th
            class="clickable"
            scope="col"
            v-if="columns.includes('Group')"
            v-on:click="changeSort('group')"
          >
            Group
            <span
              :class="{'oi-arrow-thick-top': !reversed, 'oi-arrow-thick-bottom': reversed}"
              class="oi"
              v-if="sortBy == 'group'"
            ></span>
          </th>
          <th
            class="clickable"
            scope="col"
            v-if="columns.includes('Start Time')"
            v-on:click="changeSort('start')"
          >
            Start Time
            <span
              :class="{'oi-arrow-thick-top': !reversed, 'oi-arrow-thick-bottom': reversed}"
              class="oi"
              v-if="sortBy == 'start'"
            ></span>
          </th>
          <th
            class="clickable"
            scope="col"
            v-if="columns.includes('End Time')"
            v-on:click="changeSort('end')"
          >
            End Time
            <span
              :class="{'oi-arrow-thick-top': !reversed, 'oi-arrow-thick-bottom': reversed}"
              class="oi"
              v-if="sortBy == 'end'"
            ></span>
          </th>
          <th
            class="clickable"
            scope="col"
            v-if="columns.includes('Nodes')"
            v-on:click="changeSort('nodes')"
          >
            Nodes
            <span
              :class="{'oi-arrow-thick-top': !reversed, 'oi-arrow-thick-bottom': reversed}"
              class="oi"
              v-if="sortBy == 'nodes'"
            ></span>
          </th>
          <th
            class="clickable"
            scope="col"
            v-if="columns.includes('Range')"
            v-on:click="changeSort('range')"
          >
            Range
            <span
              :class="{'oi-arrow-thick-top': !reversed, 'oi-arrow-thick-bottom': reversed}"
              class="oi"
              v-if="sortBy == 'range'"
            ></span>
          </th>
          <th scope="col">&nbsp;</th> <!-- Buttons column -->
        </tr>
      </thead>
      <tbody>
        <template v-for="r in reservations">
          <reservation-table-row
            v-bind:columns="columns"
            v-bind:reservation="r"
            v-if="r.Owner != ''"
            v-on:res-action="(...args) => $emit('res-action', ...args)"
          ></reservation-table-row>
        </template>
      </tbody>
    </table>
  `;

  window.ReservationTable = {
    template: template,

    components: {
      ReservationTableRow,
    },

    props: {
      filter: {
        type: String,
      },
      columns: {
        type: Array,
      },
    },

    data() {
      return {
        sortBy: 'name',
        reversed: false,
      };
    },

    computed: {
      reservations() {
        let sortFunc = null;
        switch (this.sortBy) {
          case 'name':
            sortFunc = sortHelper((x) => x.Name.toUpperCase());
            break;
          case 'owner':
            sortFunc = sortHelper((x) => x.Owner.toUpperCase());
            break;
          case 'group':
            sortFunc = sortHelper((x) => x.Group.toUpperCase());
            break;
          case 'start':
            sortFunc = sortHelper((x) => x.StartInt);
            break;
          case 'end':
            sortFunc = sortHelper((x) => x.EndInt);
            break;
          case 'nodes':
            sortFunc = sortHelper((x) => x.Nodes.length);
            break;
          case 'range':
            sortFunc = sortHelper((x) => x.Nodes[0]);
            break;
        }

        const clone = $.extend(true, [], this.$store.getters.reservations);
        const sorted = clone.sort(sortFunc);
        const filtered = sorted.filter((x) => {
          let include = false;
          [x.Name, x.Owner].forEach((d) => {
            if (d) {
              include = include || d.toString().toLowerCase().includes(this.filter.toLowerCase());
            }
          });
          include = include || x.Nodes.includes(+this.filter);
          include = include || x.Range == this.filter;

          const singleNodeRange = this.filter.match(/^.+\[(\d+)\]$/);
          if (singleNodeRange) {
            const node = singleNodeRange[1];
            include = include || x.Nodes.includes(+node);
          }

          return include;
        });
        return this.reversed ? filtered.reverse() : filtered;
      },
    },

    methods: {
      changeSort(by) {
        if (this.sortBy == by) {
          this.reversed = !this.reversed;
        } else {
          this.sortBy = by;
        }
      },
    },
  };

  function sortHelper(getter) {
    return (a, b) => {
      const gA = getter(a);
      const gB = getter(b);

      if (gA < gB) {
        return -1;
      }

      if (gA > gB) {
        return 1;
      }

      return 0; // equal
    };
  }
})();
