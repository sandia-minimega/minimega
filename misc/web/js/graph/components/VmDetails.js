/* global _ */

(function() {
  // VmDetails is a table containing details about one VM.
  //
  // Component Properties:
  //
  //    - vm: An Object; the VM we're describing
  //
  // VM Object Properties:
  //
  //     Each property of the "vm" object becomes a row in the VmDetails
  //     table. The property name is in the first column, and the
  //     property value is in the second column.
  //
  // Example:
  //
  //     <vm-details :vm="selectedVm"></vm-details>
  //

  const template = `
      <table class="table table-striped table-bordered">
          <thead>
              <tr>
                  <th>Field</th>
                  <th>Value</th>
              </tr>
          </thead>
          <tbody>
              <tr v-for="field in fields">
                  <td>{{ field | formatVMField }}</td>
                  <td>{{ vm[field] }}</td>
              </tr>
          </tbody>
      </table>
      `;

  window.VmDetails = {
    template: template,

    // Filters used in this Vue template
    filters: {
      formatVMField,
    },

    // Component properties. This is the data that is passed to the
    // vm-details tag.
    //
    // These values should be treated as read-only.
    props: {
      vm: {
        type: Object,
      },
    },

    // Computed values are recomputed whenever dependencies change. If
    // dependencies don't change, the cached return value is used.
    computed: {
      // The sorted Array of Object properties to display in the
      // rows of the table
      fields() {
        let keys = _.keys(this.vm);
        return _.sortBy(keys, _.identity);
      },
    },
  };
})();
