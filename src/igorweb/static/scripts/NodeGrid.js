'use strict';

(function() {
  var template = ''
    + '<div class="col">'
    + '  <div'
    + '    class="card mx-auto text-center"'
    + '    id="nodegridcard"'
    + '    style="background-color: #e7ccff; border: none;"'
    + '  >'
    + '    <div class="card-body">'
    + '      <div class="row" id="nodegrid" style="margin: 0.5em; min-height: 400px;">'
    + '        <div class="col" style="padding: 0" v-for="c in columns">'
    + '          <div class="list-group">'
    + '            <template v-for="r in rows">'
    + '              <node'
    + '                v-bind:id="getNodeInfo(c, r)[\'NodeID\']"'
    + '                v-bind:node-info="getNodeInfo(c, r)"'
    + '              ></node>'
    + '            </template>'
    + '          </div>'
    + '        </div>'
    + '      </div>'
    + '    </div>'
    + '  </div>'
    + '</div>';
  window.NodeGrid = {
    template: template,
    components: {
      Node: Node,
    },
    mounted: function mounted() {
      var _this = this;

      $('.node').on('mousedown', function(event) {
        _this.selection.start = event.target['id'];
        $('.node').on('mouseover', function(event) {
          _this.selection.end = event.target['id'];
          var min = parseInt(_this.selection.start, 10);
          var max = parseInt(_this.selection.end, 10);

          if (min > max) {
            min = parseInt(_this.selection.end, 10);
            max = parseInt(_this.selection.start, 10);
          }

          var nodes = [];

          for (var i = min; i <= max; i++) {
            nodes.push(i);
          }

          _this.$store.dispatch('selectNodes', nodes);
        });
        return false;
      });
      $(window).on('mouseup', function(event) {
        $('.node').off('mouseover');
        _this.selection.start = null;
        _this.selection.end = null;
      });
    },
    data: function data() {
      return {
        selection: {
          start: null,
          end: null,
        },
      };
    },
    methods: {
      getNodeInfo: function getNodeInfo(column, row) {
        var start = this.$store.getters.startNode;
        var width = this.$store.getters.rackWidth;
        var index = start + (row * width + column % width);
        return this.$store.getters.nodes[index];
      },
      numCols: function numCols() {
        return this.$store.getters.rackWidth;
      },
      numRows: function numRows() {
        return Math.ceil(this.$store.getters.nodeCount / this.$store.getters.rackWidth);
      },
    },
    computed: {
      columns: function columns() {
        var a = [];

        for (var i = 0; i < this.numCols(); i++) {
          a.push(i);
        }

        return a;
      },
      rows: function rows() {
        var a = [];

        for (var j = 0; j < this.numRows(); j++) {
          a.push(j);
        }

        return a;
      },
    },
  };
})();
