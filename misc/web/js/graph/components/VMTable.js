/* global _ */

(function() {
  // VmTable is a table containing rows of VM information. The columns
  // of the table are sortable and hide-able. Additionally, clicking on
  // a VM's "details" button opens a VM detail modal.
  //
  // Component Properties:
  //
  //   - fields: An Array of the fields (columns) to show in the table
  //   - initial-key: The column to sort by initially
  //   - initial-reverse: Sorts the initial-key column in descending order
  //   - title: The h3 title to show
  //   - vm-list: An Array of VM info; the VMs to show in the table
  //   - disable-details: Hides the "Details" column
  //
  // Examples:
  //
  //   <vm-table
  //     :fields="['name', 'host', 'state', 'vlan']"
  //     :vmList="vmsFor(vlan.name)"
  //     :title="Some Cool VMs">
  //   </vm-table>

  const template = `
      <div>
          <div class="btn-group pull-right">
              <button type="button"
                      class="btn btn-default"
                      v-bind:class="{ active: isVisible(field) }"
                      v-for="field in fields"
                      v-on:click="press(field)">
                  {{ field | formatVMField }}
              </button>
          </div>

          <h3 v-if="title">{{ title }}</h3>

          <table class="table table-striped table-bordered">
              <thead>
                  <tr>
                      <th v-for="field in visible"
                          v-on:click="toggleSort(field)">
                          {{ field | formatVMField }}
                          <span v-if="field === sortKey">
                              <i v-if="reverse" class="fa fa-arrow-up"></i>
                              <i v-if="!reverse" class="fa fa-arrow-down"></i>
                          </span>
                      </th>
                      <th v-if="!disableDetails"
                          style="width:0;">
                          Info
                      </th>
                  </tr>
              </thead>
              <tbody>
                  <tr v-for="vm in vms">
                      <td v-for="field in visible">
                          {{ vm[field] }}
                      </td>
                      <td v-if="!disableDetails">
                          <button
                            class="btn btn-info"
                            v-on:click="showDetails(vm)">
                              <i class="fa fa-info-circle"></i>
                          </button>
                      </td>
                  </tr>
              </tbody>
          </table>

          <mm-modal
            v-if="selectedVm != null && !disableDetails"
            :title="selectedVm.name"
            v-on:closed="hideDetails()">
                <vm-details :vm="selectedVm"></vm-details>
          </mm-modal>
      </div>
      `;

  window.VmTable = {
    template: template,

    // Other components used by this Vue template
    components: {
      MmModal,
      VmDetails,
    },

    // Filters used in this Vue template
    filters: {
      formatVMField,
    },

    // Local data for VmTable
    data() {
      return {
        // An Array of field names (columns) that we're hiding
        hidden: () => [],

        // The field name (column) that we're sorting by
        sortKey: this.initialKey,

        // Set to true if we're sorting in descending order
        reverse: this.initialReverse,

        // The selected VM has a modal displaying VM details
        selectedVm: null,
      };
    },

    // Convenience methods!
    methods: {
      // Called when a field name button is clicked. Toggles hiding
      // columns in the table.
      press(fieldName) {
        if (_.contains(this.hidden, fieldName)) {
          // If it's already hidden, show it by removing it from
          // the list of hidden fields
          this.hidden = _.difference(this.hidden, [fieldName]);
        } else {
          // Otherwise, hide it by adding it to the list of
          // hidden fields
          this.hidden = _.union(this.hidden, [fieldName]);
        }
      },

      // Returns true if the given field name is hidden
      isVisible(fieldName) {
        return !_.contains(this.hidden, fieldName);
      },

      // Called when a column header is clicked.
      toggleSort(fieldName) {
        if (this.sortKey === fieldName) {
          // If already sorting by this column, reverse the sort
          // direction.
          this.reverse = !this.reverse;
        } else {
          // If not sorting by the clicked column, sorts by the
          // clicked colum
          this.sortKey = fieldName;
        }
      },

      // Called when a VM info button (VM details) is clicked. Sets
      // the selected VM and shows VM details in a modal
      showDetails(vm) {
        this.selectedVm = vm;
      },

      // Hides the VM details modal
      hideDetails() {
        this.selectedVm = null;
      },
    },

    // Component properties. This is the data that is passed to the
    // vm-table tag.
    //
    // These values should be treated as read-only.
    props: {
      // The fields (columns) to show in the table
      fields: {
        type: Array,
        default: () => ['host', 'name', 'state', 'uptime', 'type',
          'vcpus', 'memory', 'disk', 'vlan', 'ip',
          'ip6', 'tap', 'tags', 'cc_active', 'vnc_port'],
      },

      // Initially sort the table by this field
      initialKey: {
        type: String,
        default: 'name',
      },

      // Initially sort in descending order
      initialReverse: {
        type: Boolean,
        default: false,
      },

      // An optional title to display above the table
      title: {
        type: String,
      },

      // A list of VMs to display in the table
      vmList: {
        type: Array,
      },

      // Disables the VM details column (hides the details button)
      disableDetails: {
        type: Boolean,
      },
    },

    // Computed values are recomputed whenever dependencies change. If
    // dependencies don't change, the cached return value is used.
    computed: {
      // The sorted Array of VMs (in order for display)
      vms() {
        // Wrap values because underscore is so dandy
        let vms = _(this.vmList);

        // Sort vms by the key
        let sorted = vms.sortBy((vm) => vm[this.sortKey]);

        return this.reverse ? sorted.reverse() : sorted;
      },

      // Returns an Array of the visible (non-hidden) columns
      visible() {
        return _.difference(this.fields, this.hidden);
      },
    },
  };
})();
