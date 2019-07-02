(function() {
  const template = `
    <table class="table table-hover table-borderless mdl">
      <thead class="mdl">
        <tr class="mdl">
          <!-- Reservation table headers with sorting arrows -->
          <th
            class="mdl restableheader clickable"
            id="rtname"
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
            class="mdl restableheader clickable"
            id="rtowner"
            scope="col"
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
            class="mdl restableheader clickable"
            id="rtgroup"
            scope="col"
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
            class="mdl restableheader clickable"
            id="rtstart"
            scope="col"
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
            class="mdl restableheader clickable"
            id="rtend"
            scope="col"
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
            class="mdl restableheader clickable"
            id="rtnumber"
            scope="col"
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
            class="mdl restableheader clickable"
            id="rtnumber"
            scope="col"
            v-on:click="changeSort('range')"
          >
            Range
            <span
              :class="{'oi-arrow-thick-top': !reversed, 'oi-arrow-thick-bottom': reversed}"
              class="oi"
              v-if="sortBy == 'range'"
            ></span>
          </th>
        </tr>
      </thead>
      <tbody class="mdl" id="res_table">
        <template v-for="r in reservations">
          <reservation-table-row
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
              include = include || d.toString().includes(this.filter);
            }
          });
          include = include || x.Nodes.includes(+this.filter);
          include = include || x.Range == this.filter;

          const single_node_range = this.filter.match(/^.+\[(\d+)\]$/);
          if (single_node_range) {
            const node = single_node_range[1];
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
