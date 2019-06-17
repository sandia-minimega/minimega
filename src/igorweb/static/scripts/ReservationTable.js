(function() {
  const template = `
    <table class="table table-hover table-borderless mdl">
      <thead class="mdl">
        <tr class="mdl">
          <!-- Reservation table headers with sorting arrows -->
          <th id="rtname" class="mdl restableheader clickable" scope="col" v-on:click="changeSort('name')">
            Name <span v-if="sortBy == 'name'"><b v-if="!reversed">&uarr;</b><b v-if="reversed">&darr;</b></span>
          </th>
          <th id="rtowner" class="mdl restableheader clickable" scope="col" v-on:click="changeSort('owner')">
            Owner <span v-if="sortBy == 'owner'"><b v-if="!reversed">&uarr;</b><b v-if="reversed">&darr;</b></span>
          </th>
          <th id="rtstart" class="mdl restableheader clickable" scope="col" v-on:click="changeSort('start')">
            Start Time <span v-if="sortBy == 'start'"><b v-if="!reversed">&uarr;</b><b v-if="reversed">&darr;</b></span>
          </th>
          <th id="rtend" class="mdl restableheader clickable" scope="col" v-on:click="changeSort('end')">
            End Time <span v-if="sortBy == 'end'"><b v-if="!reversed">&uarr;</b><b v-if="reversed">&darr;</b></span>
          </th>
          <th id="rtnumber" class="mdl restableheader clickable" scope="col" v-on:click="changeSort('nodes')">
            Nodes <span v-if="sortBy == 'nodes'"><b v-if="!reversed">&uarr;</b><b v-if="reversed">&darr;</b></span>
          </th>
          <th id="rtnumber" class="mdl restableheader clickable" scope="col" v-on:click="changeSort('range')">
            Range <span v-if="sortBy == 'range'"><b v-if="!reversed">&uarr;</b><b v-if="reversed">&darr;</b></span>
          </th>
        </tr>
      </thead>
      <tbody id="res_table" class="mdl">
        <template v-for="r in reservations">
          <reservation-table-row
            v-if="r.Owner != ''"
            v-bind:reservation="r">
          </reservation-table-row>
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
