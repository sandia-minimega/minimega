import {VmTable} from './VMTable.js';

const template = `
    <div>
      <div class="form-inline pull-right">
        <div class="form-group">
          <div class="input-group">
            <input type="text" class="form-control" placeholder="Query" v-model="searchQuery">
            <div class="input-group-addon">
              <i class="fa fa-search"></i>
            </div>
          </div>
          <div class="btn-group">
            <button v-bind:class="{ disabled: this.expanded.length >= this.vlans.length }" class="btn btn-default" v-on:click="expandAll()">
              <i class="fa fa-expand"></i>
            </button>
            <button v-bind:class="{ disabled: this.expanded.length <= 0 }" class="btn btn-default" v-on:click="collapseAll()">
              <i class="fa fa-compress"></i>
            </button>
          </div>
        </div>
      </div>

      <h3>VLANs</h3>
      <div v-if="vlans.length == 0" class="well text-center">
        <i>No VLANs to show.</i>
      </div>
      <div class="panel-group">
        <div class="panel panel-default" v-for="vlan in matchingVlans">
          <template>
            <div class="panel-heading" v-on:click="toggleCollapse(vlan.name)">
              <h4 class="panel-title">
                <span>
                  {{ vlan.name }}
                </span>
              </h4>
            </div>
            <div class="panel-collapse collapse" v-bind:class="{ in: isExpanded(vlan.name) }">
              <div class="panel-body">
                  <!-- TODO: Consider using v-if to render lazily -->
                  <vm-table
                      :fields="['name', 'host', 'state', 'vlan']"
                      :vmList="vmsFor(vlan.name)"
                      :title="vlan.name">
                  </vm-table>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>
    `;

export var VlanListGroup = {
    template: template,

    components: {
        VmTable,
    },

    data() {
        return {
            expanded: () => [],
            searchQuery: "",
        };
    },

    methods: {
        isExpanded(vlanName) {
            return _.contains(this.expanded, vlanName);
        },

        toggleCollapse(vlanName) {
            if (this.isExpanded(vlanName)) {
                this.hide(vlanName);
            } else {
                this.show(vlanName);
            }
        },

        show(vlanName) {
            this.expanded = _.union(this.expanded, [vlanName]);
        },

        hide(vlanName) {
            this.expanded = _.difference(this.expanded, [vlanName]);
        },

        vmsFor(vlanName) {
            return this.$store.getters.vlans[vlanName].vms;
        },

        expandAll() {
            this.expanded = _.map(this.vlans, (v) => v.name);
        },

        collapseAll() {
            this.expanded = [];
        },
    },

    computed: {
        vlans() {
            return _.sortBy(this.$store.getters.vlans, v => v.number);
        },

        matchingVlans() {
            return _.filter(this.vlans, (v) => {
                return s.contains(v.name, this.searchQuery);
            });
        }
    }
};
