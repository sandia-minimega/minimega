(function() {
  const template = `
    <!-- Container of node grid, key, and reservation table -->
    <div class="container-fluid" style="margin-top:30px;">
      <div class="row">
        <div class="col-12 col-xl-6">
          <node-grid></node-grid>
        </div>
        <div class="col-12 col-xl-6">
          <div class="row">
            <div class="col-3"></div>
            <!-- offset -->
            <div class="col-6">
              <key-card></key-card>
            </div>
            <div class="col-12">
              <div class="card mx-auto">
                <div class="card-body">
                  <div class="input-group">
                    <input
                      class="form-control"
                      placeholder="Filter Reservations"
                      type="text"
                      v-model="searchText"
                    >
                    <button
                      class="btn"
                      v-if="searchText != ''"
                      v-on:click="clearFilter()"
                    >Clear</button>
                  </div>
                  <div
                    class="form-group"
                  >
                    <div
                      class="form-check form-check-inline"
                      v-for="col in columns"
                    >
                      <input
                        class="form-check-input"
                        type="checkbox"
                        :value="col"
                        v-model="shownColumns"
                      >
                      <label
                        class="form-check-label font-weight-bold"
                      >{{ col }}</label>
                    </div>
                  </div>
                  <div class="row" id="table" style="margin: 0.5em; opacity: 1;">
                    <reservation-table
                      :filter="searchText"
                      :columns="shownColumns"
                      v-on:res-action="(...args) => $emit('res-action', ...args)"
                    ></reservation-table>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  `;

  window.ReservationInfo = {
    template: template,

    components: {
      ReservationTable,
      NodeGrid,
      KeyCard,
    },

    data() {
      return {
        searchText: '',
        columns: ['Owner', 'Group', 'Start Time', 'End Time', 'Nodes', 'Range'],
        shownColumns: ['Owner', 'End Time', 'Range'],
      };
    },

    methods: {
      clearFilter() {
        this.searchText = '';
      },
    },
  };
})();
