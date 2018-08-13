/* global _, s */

import {VmTable} from './VMTable.js';

// VLANListGroup is a Bootstrap accordion
// (https://getbootstrap.com/docs/3.3/javascript/#collapse-example-accordion)
// that groups VMs by VLAN. By default, each VLAN accordion is
// collapsed. Clicking a VLAN accordion shows a VmTable that lists all
// members of the VLAN. A search field at the top allows users to
// filter the list of VLAN names.
//
//
// Examples:
//
//     <vlan-list-group></vlan-list-group>
//

const template = `
    <div>
      <div class="form-inline pull-right">
        <div class="form-group">
          <div class="input-group">
            <input
              type="text"
              class="form-control"
              placeholder="Query"
              v-model="searchQuery">

            <div class="input-group-addon">
              <i class="fa fa-search"></i>
            </div>
          </div>
          <div class="btn-group">
            <button
              v-bind:class="{ disabled: this.expanded.length >= this.vlans.length }"
              class="btn btn-default"
              v-on:click="expandAll()">
                <i class="fa fa-expand"></i>
            </button>
            <button
              v-bind:class="{ disabled: this.expanded.length <= 0 }"
              class="btn btn-default"
              v-on:click="collapseAll()">
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

export const VlanListGroup = {
  template: template,

  // Other components used by this Vue template
  components: {
    VmTable,
  },

  // Local data for the VlanListGroup
  data() {
    return {
      // An Array of strings. The names of VLANs whose
      // accordions are expanded.
      expanded: () => [],

      // A string. The search string entered into the filter field.
      searchQuery: '',
    };
  },

  // Convenience methods!
  methods: {
    // Returns true if the accordion for the given VLAN should be
    // expanded
    isExpanded(vlanName) {
      return _.contains(this.expanded, vlanName);
    },

    // Toggles a VLAN accordion between expanded or collapsed
    toggleCollapse(vlanName) {
      if (this.isExpanded(vlanName)) {
        this.hide(vlanName);
      } else {
        this.show(vlanName);
      }
    },

    // Expands the VLAN accordion for the given VLAN name
    show(vlanName) {
      this.expanded = _.union(this.expanded, [vlanName]);
    },

    // Collapses the VLAN accordion for the given VLAN name
    hide(vlanName) {
      this.expanded = _.difference(this.expanded, [vlanName]);
    },

    // Returns an Array of all VMs connected to the given VLAN
    vmsFor(vlanName) {
      return this.$store.getters.vlans[vlanName].vms;
    },

    // Expands all VLAN accordions
    expandAll() {
      this.expanded = _.map(this.vlans, (v) => v.name);
    },

    // Collapses all VLAN accordions
    collapseAll() {
      this.expanded = [];
    },
  },

  // Computed values are recomputed whenever dependencies change. If
  // dependencies don't change, the cached return value is used.
  computed: {
    // Returns an Array of VLANs sorted by VLAN number
    vlans() {
      return _.sortBy(this.$store.getters.vlans, (v) => v.number);
    },

    // Returns an Array of VLANs whose names contain the
    // user-provided search query
    matchingVlans() {
      return _.filter(this.vlans, (v) => {
        return s.contains(v.name, this.searchQuery);
      });
    },
  },
};
