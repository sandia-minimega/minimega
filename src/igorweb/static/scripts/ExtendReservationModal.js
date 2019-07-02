(function() {
  const template = `
    <div>
      <!-- Extend reservation modal -->
      <div
        aria-hidden="true"
        aria-labelledby="Extend Reservation"
        class="modal fade mdl"
        id="extendmodal"
        ref="modal"
        role="dialog"
        tabindex="-1"
      >
        <div class="modal-dialog modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl">
              <h5 class="modal-title text-center col-12 mdl" id="emodaltitle">
                <b class="mdl">Extend Reservation</b>
              </h5>
              <button
                aria-label="Close"
                class="close mdl"
                data-dismiss="modal"
                style="position: absolute; right: 15px; top: 10px;"
                type="button"
              >
                <span aria-hidden="true" class="mdl">&times;</span>
              </button>
            </div>
            <!-- Form with all of the fields -->
            <div class="modal-body m-3 mdl">
              <form class="mdl">
                <!-- Reservation name, -r -->
                <div class="form-group mdl">
                  <div
                    class="input-group mdl"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Reservation name"
                  >
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code class="mdl" id="edashrcode">-r</code>
                      </div>
                    </div>
                    <input
                      autofocus
                      class="edash form-control mdl"
                      id="edashr"
                      placeholder="Reservation name"
                      type="text"
                      v-model="resName"
                    >
                  </div>
                </div>
                <i class="mb-2 mdl">Optional:</i>
                <!-- Extension length, -t, optional, default is 60m -->
                <div
                  class="mb-4 mdl"
                  style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;"
                >
                  <div class="form-group mdl">
                    <div
                      class="input-group mdl"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="Time denominations should be specified in days(d), hours(h), and minutes(m), in that order. Unitless numbers are treated as minutes. Days are defined as 24*60 minutes. Example: To make a reservation for 7 days: -t 7d. To make a reservation for 4 days, 6 hours, 30 minutes: -t 4d6h30m (default = 60m)."
                    >
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl">
                          <code
                            class="mdl"
                            id="edashtcode"
                            style="color: royalblue;"
                          >-t</code>
                        </div>
                      </div>
                      <input
                        class="edash form-control mdl"
                        id="edasht"
                        placeholder="Extension length"
                        type="text"
                        v-model="timeRange"
                        value="60m"
                      >
                    </div>
                  </div>
                </div>
              </form>
              <!-- Command box, updates command text as user constructs it from filling fields.
              Shows exactly what will be run on igor-->
              <div class="card commandline mdl">
                <code
                  class="mdl"
                  id="ecommandline"
                  style="color: seagreen;"
                >{{ command }}</code>
              </div>
            </div>
            <!-- Buttons at bottom of modal -->
            <div class="modal-footer m-3 mdl">
              <!-- Cancel, exits modal, only shows on main reservation page -->
              <button
                class="modalbtn igorbtn btn btn-secondary mr-auto mdl cancel"
                data-dismiss="modal"
                type="button"
              >Cancel</button>
              <button
                :disabled="!validForm"
                class="modalbtn extendmodalgobtn igorbtn btn btn-primary mdl modalcommand"
                id="extend"
                style="background-color: #a975d6; border-color: #a975d6;"
                type="button"
                v-on:click="extendReservation()"
              >
                <span class="mdl mdlcmdtext">Extend</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <loading-modal
        body="This may take some time..."
        header="Extending Reservation"
        ref="loadingModal"
      ></loading-modal>
    </div>
  `;

  window.ExtendReservationModal = {
    template: template,

    components: {
      LoadingModal,
    },

    data() {
      return {
        resName: '',
        timeRange: '60m',
      };
    },

    beforeDestroy() {
      $(this.$refs['modal']).modal('hide');
    },

    computed: {
      validForm() {
        return this.resName !== '';
      },

      command() {
        let time = '';
        if (this.timeRange) {
          time = ` -t ${this.timeRange}`;
        }
        return `igor extend -r ${this.resName}${time}`;
      },
    },

    methods: {
      show() {
        const res = this.$store.state.selectedReservation;
        if (res) {
          this.resName = res.Name;
        }

        $(this.$refs['modal']).modal('show');
      },

      hide() {
        $(this.$refs['modal']).modal('hide');
      },

      showLoading() {
        this.$refs['loadingModal'].show();
      },

      hideLoading() {
        setTimeout(this.$refs['loadingModal'].hide, 500);
      },

      extendReservation() {
        this.hide();
        this.showLoading();

        $.get(
            'run/',
            {run: this.command},
            (data) => {
              const response = JSON.parse(data);
              this.$store.commit('setAlert', response.Message);
              this.hideLoading();
            }
        );
      },
    },
  };
})();
