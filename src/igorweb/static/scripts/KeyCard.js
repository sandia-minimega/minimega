'use strict';

(function() {
  const template = ''
    + '<div class="card" style="margin-bottom:10px;">'
    + '  <div class="card-body" style="padding:0px;">'
    + '    <table class="table table-borderless">'
    + '      <tbody>'
    + '        <tr>'
    + '          <td></td>'
    + '          <td'
    + '            class="key available clickable tdhover text-center"'
    + '            v-on:click.stop="select(\'available\', null);"'
    + '          >Available</td>'
    + '          <td'
    + '            class="key reserved clickable tdhover text-center"'
    + '            v-on:click.stop="select(\'reserved\', null);"'
    + '          >Reserved</td>'
    + '        </tr>'
    + '        <tr>'
    + '          <td'
    + '            class="key up clickable tdhover text-right"'
    + '            v-on:click.stop="select(null, \'up\');"'
    + '          >Powered On</td>'
    + '          <td'
    + '            class="key available up clickable tdhover"'
    + '            v-on:click.stop="select(\'available\', \'up\');"'
    + '          >'
    + '            <div class="mx-auto keycolor available up unselected"></div>'
    + '          </td>'
    + '          <td'
    + '            class="key reserved up clickable tdhover"'
    + '            v-on:click.stop="select(\'reserved\', \'up\')"'
    + '          >'
    + '            <div class="mx-auto keycolor reserved up unselected"></div>'
    + '          </td>'
    + '        </tr>'
    + '        <tr>'
    + '          <td'
    + '            class="key down clickable tdhover text-right"'
    + '            v-on:click.stop="select(null, \'down\');"'
    + '          >Powered Off</td>'
    + '          <td'
    + '            class="key available down clickable tdhover"'
    + '            v-on:click.stop="select(\'available\', \'down\')"'
    + '          >'
    + '            <div class="mx-auto keycolor available down unselected"></div>'
    + '          </td>'
    + '          <td'
    + '            class="key reserved down clickable tdhover"'
    + '            v-on:click.stop="select(\'reserved\', \'down\')"'
    + '          >'
    + '            <div class="mx-auto keycolor reserved down unselected"></div>'
    + '          </td>'
    + '        </tr>'
    + '      </tbody>'
    + '    </table>'
    + '  </div>'
    + '</div>'
    + '';
  window.KeyCard = {
    template: template,
    methods: {
      select: function select(availability, power) {
        const nodes = Object.values(this.$store.getters.nodes);
        let selected = nodes;

        if (availability == 'available') {
          selected = nodes.filter(function(x) {
            return x.Reservation == null;
          });
        }

        if (availability == 'reserved') {
          selected = nodes.filter(function(x) {
            return x.Reservation != null;
          });
        }

        if (power == 'up') {
          selected = selected.filter(function(x) {
            return x.Up;
          });
        }

        if (power == 'down') {
          selected = selected.filter(function(x) {
            return !x.Up;
          });
        }

        selected = selected.map(function(x) {
          return x.NodeID;
        });
        this.$store.dispatch('selectNodes', selected);
      },
    },
  };
})();
