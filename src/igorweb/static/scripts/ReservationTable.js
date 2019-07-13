'use strict';

(function() {
  var template = ''
    + '<table class="table table-hover table-borderless">'
    + '  <thead>'
    + '    <tr>'
    + '      <!-- Reservation table headers with sorting arrows -->'
    + '      <th class="clickable" scope="col" v-on:click="changeSort(\'name\')">'
    + '        Name'
    + '        <span'
    + '          :class="{\'oi-arrow-thick-top\': !reversed, \'oi-arrow-thick-bottom\': reversed}"'
    + '          class="oi"'
    + '          v-if="sortBy == \'name\'"'
    + '        ></span>'
    + '      </th>'
    + '      <th class="clickable" scope="col" v-on:click="changeSort(\'owner\')">'
    + '        Owner'
    + '        <span'
    + '          :class="{\'oi-arrow-thick-top\': !reversed, \'oi-arrow-thick-bottom\': reversed}"'
    + '          class="oi"'
    + '          v-if="sortBy == \'owner\'"'
    + '        ></span>'
    + '      </th>'
    + '      <th class="clickable" scope="col" v-on:click="changeSort(\'group\')">'
    + '        Group'
    + '        <span'
    + '          :class="{\'oi-arrow-thick-top\': !reversed, \'oi-arrow-thick-bottom\': reversed}"'
    + '          class="oi"'
    + '          v-if="sortBy == \'group\'"'
    + '        ></span>'
    + '      </th>'
    + '      <th class="clickable" scope="col" v-on:click="changeSort(\'start\')">'
    + '        Start Time'
    + '        <span'
    + '          :class="{\'oi-arrow-thick-top\': !reversed, \'oi-arrow-thick-bottom\': reversed}"'
    + '          class="oi"'
    + '          v-if="sortBy == \'start\'"'
    + '        ></span>'
    + '      </th>'
    + '      <th class="clickable" scope="col" v-on:click="changeSort(\'end\')">'
    + '        End Time'
    + '        <span'
    + '          :class="{\'oi-arrow-thick-top\': !reversed, \'oi-arrow-thick-bottom\': reversed}"'
    + '          class="oi"'
    + '          v-if="sortBy == \'end\'"'
    + '        ></span>'
    + '      </th>'
    + '      <th class="clickable" scope="col" v-on:click="changeSort(\'nodes\')">'
    + '        Nodes'
    + '        <span'
    + '          :class="{\'oi-arrow-thick-top\': !reversed, \'oi-arrow-thick-bottom\': reversed}"'
    + '          class="oi"'
    + '          v-if="sortBy == \'nodes\'"'
    + '        ></span>'
    + '      </th>'
    + '      <th class="clickable" scope="col" v-on:click="changeSort(\'range\')">'
    + '        Range'
    + '        <span'
    + '          :class="{\'oi-arrow-thick-top\': !reversed, \'oi-arrow-thick-bottom\': reversed}"'
    + '          class="oi"'
    + '          v-if="sortBy == \'range\'"'
    + '        ></span>'
    + '      </th>'
    + '      <th scope="col">&nbsp;</th> <!-- Buttons column -->'
    + '    </tr>'
    + '  </thead>'
    + '  <tbody>'
    + '    <template v-for="r in reservations">'
    + '      <reservation-table-row'
    + '        v-bind:reservation="r"'
    + '        v-if="r.Owner != \'\'"'
    + '        v-on:res-action="(...args) => $emit(\'res-action\', ...args)"'
    + '      ></reservation-table-row>'
    + '    </template>'
    + '  </tbody>'
    + '</table>';
  window.ReservationTable = {
    template: template,
    components: {
      ReservationTableRow: ReservationTableRow,
    },
    props: {
      filter: {
        type: String,
      },
    },
    data: function data() {
      return {
        sortBy: 'name',
        reversed: false,
      };
    },
    computed: {
      reservations: function reservations() {
        var _this = this;

        var sortFunc = null;

        switch (this.sortBy) {
          case 'name':
            sortFunc = sortHelper(function(x) {
              return x.Name.toUpperCase();
            });
            break;

          case 'owner':
            sortFunc = sortHelper(function(x) {
              return x.Owner.toUpperCase();
            });
            break;

          case 'group':
            sortFunc = sortHelper(function(x) {
              return x.Group.toUpperCase();
            });
            break;

          case 'start':
            sortFunc = sortHelper(function(x) {
              return x.StartInt;
            });
            break;

          case 'end':
            sortFunc = sortHelper(function(x) {
              return x.EndInt;
            });
            break;

          case 'nodes':
            sortFunc = sortHelper(function(x) {
              return x.Nodes.length;
            });
            break;

          case 'range':
            sortFunc = sortHelper(function(x) {
              return x.Nodes[0];
            });
            break;
        }

        var clone = $.extend(true, [], this.$store.getters.reservations);
        var sorted = clone.sort(sortFunc);
        var filtered = sorted.filter(function(x) {
          var include = false;
          [x.Name, x.Owner].forEach(function(d) {
            if (d) {
              include = include || d.toString().includes(_this.filter);
            }
          });
          include = include || x.Nodes.includes(+_this.filter);
          include = include || x.Range == _this.filter;

          var single_node_range = _this.filter.match(/^.+\[(\d+)\]$/);

          if (single_node_range) {
            var node = single_node_range[1];
            include = include || x.Nodes.includes(+node);
          }

          return include;
        });
        return this.reversed ? filtered.reverse() : filtered;
      },
    },
    methods: {
      changeSort: function changeSort(by) {
        if (this.sortBy == by) {
          this.reversed = !this.reversed;
        } else {
          this.sortBy = by;
        }
      },
    },
  };

  function sortHelper(getter) {
    return function(a, b) {
      var gA = getter(a);
      var gB = getter(b);

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
