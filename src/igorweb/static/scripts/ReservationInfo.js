(function() {
  const template = `
    <!-- Container of node grid, key, and reservation table -->
    <div class="container-fluid" style="margin-top:10px;">
      <div class="row">
        <div class="col-12 col-lg-6">
          <node-grid></node-grid>
        </div>
        <div class="col-12 col-lg-6">
          <div class="row">
            <div class="col-3 col-lg-3"></div>
            <!-- offset -->
            <div class="col-6 col-lg-6">
              <key-card></key-card>
            </div>
            <div class="col-12 col-lg-12">
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
                      class="btn btn-default"
                      v-if="searchText != ''"
                      v-on:click="clearFilter()"
                    >Clear</button>
                  </div>
                  <div
                    class="row mdl"
                    id="table"
                    style="margin: 0.5em; opacity: 1;"
                  >
                    <reservation-table
                      :filter="searchText"
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
      };
    },

    methods: {
      clearFilter() {
        this.searchText = '';
      },
    },
  };
})();
