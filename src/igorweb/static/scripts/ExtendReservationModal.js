(function() {
  const template = `
    <div>
      <!-- Extend reservation modal -->
      <div class="modal fade mdl" id="extendmodal" tabindex="-1" role="dialog" aria-labelledby="Extend Reservation" aria-hidden="true" ref="modal">
        <div class="modal-dialog modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl">
              <h5 class="modal-title text-center col-12 mdl" id="emodaltitle"><b class="mdl">Extend Reservation</b></h5>
              <button type="button" class="close mdl" data-dismiss="modal" aria-label="Close" style="position: absolute; right: 15px; top: 10px;">
                <span class="mdl" aria-hidden="true">&times;</span>
              </button>
            </div>
            <!-- Form with all of the fields -->
            <div class="modal-body m-3 mdl">
              <form class="mdl">
                <!-- Reservation name, -r -->
                <div class="form-group mdl">
                  <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Reservation name">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="edashrcode" class="mdl">-r</code></div>
                    </div>
                    <input id="edashr" type="text" class="edash form-control mdl" placeholder="Reservation name" autofocus v-model="resName">
                  </div>
                </div>
                <i class="mb-2 mdl">Optional:</i>
                <!-- Extension length, -t, optional, default is 60m -->
                <div class="mb-4 mdl" style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;">
                  <div class="form-group mdl">
                    <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Time denominations should be specified in days(d), hours(h), and minutes(m), in that order. Unitless numbers are treated as minutes. Days are defined as 24*60 minutes. Example: To make a reservation for 7 days: -t 7d. To make a reservation for 4 days, 6 hours, 30 minutes: -t 4d6h30m (default = 60m).">
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl"><code id="edashtcode" class="mdl" style="color: royalblue;">-t</code></div>
                      </div>
                      <input id="edasht" value="60m" type="text" class="edash form-control mdl" placeholder="Extension length" v-model="timeRange">
                    </div>
                  </div>
                </div>
              </form>
              <!-- Command box, updates command text as user constructs it from filling fields.
                   Shows exactly what will be run on igor -->
              <div class="card commandline mdl"><code id="ecommandline" class="mdl" style="color: seagreen;">{{ command }}</code></div>

            </div>
            <!-- Buttons at bottom of modal -->
            <div class="modal-footer m-3 mdl">
              <!-- Cancel, exits modal, only shows on main reservation page -->
              <button type="button" class="modalbtn igorbtn btn btn-secondary mr-auto mdl cancel" data-dismiss="modal">Cancel</button>
              <button type="button" style="background-color: #a975d6; border-color: #a975d6;" class="modalbtn extendmodalgobtn igorbtn btn btn-primary mdl modalcommand" id="extend" :disabled="!validForm" v-on:click="extendReservation()"><span class="mdl mdlcmdtext">Extend</span></button>
            </div>
          </div>
        </div>
      </div>

      <loading-modal
        ref="loadingModal"
        header="Extending Reservation"
        body="This may take some time..."
      >
      </loading-modal>
    </div>
    `;

  window.ExtendReservationModal = {
    template: template,

    components: {
      LoadingModal,
    },

    props: {
      reservation: {
        type: Object,
      },
    },

    data() {
      return {
        serverMessage: '',
        serverSuccess: true,

        resName: this.reservation.Name,
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
            let response = JSON.parse(data);
            this.$store.commit('setAlert', response.Message);
            this.hideLoading();
          }
        );
      },
    },
  };
})();
