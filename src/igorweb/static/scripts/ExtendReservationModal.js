'use strict';

(function() {
  const template = ''
    + '<div>'
    + '  <!-- Extend reservation modal -->'
    + '  <div'
    + '    aria-hidden="true"'
    + '    aria-labelledby="Extend Reservation"'
    + '    class="modal fade"'
    + '    ref="modal"'
    + '    role="dialog"'
    + '    tabindex="-1"'
    + '  >'
    + '    <div class="modal-dialog modal-dialog-centered" role="document">'
    + '      <div class="modal-content">'
    + '        <div class="modal-header m-3">'
    + '          <h5 class="modal-title text-center col-12">'
    + '            <b>Extend Reservation</b>'
    + '          </h5>'
    + '          <button'
    + '            aria-label="Close"'
    + '            class="close"'
    + '            data-dismiss="modal"'
    + '            style="position: absolute; right: 15px; top: 10px;"'
    + '            type="button"'
    + '          >'
    + '            <span aria-hidden="true">&times;</span>'
    + '          </button>'
    + '        </div>'
    + '        <!-- Form with all of the fields -->'
    + '        <div class="modal-body m-3">'
    + '          <form>'
    + '            <!-- Reservation name, -r -->'
    + '            <div class="form-group">'
    + '              <div'
    + '                class="input-group"'
    + '                data-placement="bottom"'
    + '                data-toggle="tooltip"'
    + '                title="Reservation name"'
    + '              >'
    + '                <div class="input-group-prepend">'
    + '                  <div class="input-group-text">'
    + '                    <code id="rcode">-r</code>'
    + '                  </div>'
    + '                </div>'
    + '                <input'
    + '                  autofocus'
    + '                  class="form-control"'
    + '                  id="r"'
    + '                  placeholder="Reservation name"'
    + '                  type="text"'
    + '                  v-model="resName"'
    + '                >'
    + '              </div>'
    + '            </div>'
    + '            <i class="mb-2">Optional:</i>'
    + '            <!-- Extension length, -t, optional, default is 60m -->'
    + '            <div'
    + '              class="mb-4"'
    + '              style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;"'
    + '            >'
    + '              <div class="form-group">'
    + '                <div'
    + '                  class="input-group"'
    + '                  data-placement="bottom"'
    + '                  data-toggle="tooltip"'
    + '                  title="Time denominations should be specified in days(d), hours(h), and minutes(m), in that order. Unitless numbers are treated as minutes. Days are defined as 24*60 minutes. Example: To make a reservation for 7 days: -t 7d. To make a reservation for 4 days, 6 hours, 30 minutes: -t 4d6h30m (default = 60m)."'
    + '                >'
    + '                  <div class="input-group-prepend">'
    + '                    <div class="input-group-text">'
    + '                      <code id="code" style="color: royalblue;">-t</code>'
    + '                    </div>'
    + '                  </div>'
    + '                  <input'
    + '                    class="form-control"'
    + '                    placeholder="Extension length"'
    + '                    type="text"'
    + '                    v-model="timeRange"'
    + '                    value="60m"'
    + '                  >'
    + '                </div>'
    + '              </div>'
    + '            </div>'
    + '          </form>'
    + '          <!-- Command box, updates command text as user constructs it from filling fields.'
    + '          Shows exactly what will be run on igor-->'
    + '          <div class="card commandline">'
    + '            <code style="color: seagreen;">{{ command }}</code>'
    + '          </div>'
    + '        </div>'
    + '        <!-- Buttons at bottom of modal -->'
    + '        <div class="modal-footer m-3">'
    + '          <!-- Cancel, exits modal, only shows on main reservation page -->'
    + '          <button'
    + '            class="modalbtn igorbtn btn btn-secondary mr-auto cancel"'
    + '            data-dismiss="modal"'
    + '            type="button"'
    + '          >Cancel</button>'
    + '          <button'
    + '            :disabled="!validForm"'
    + '            class="modalbtn igorbtn btn btn-primary modalcommand"'
    + '            id="extend"'
    + '            style="background-color: #a975d6; border-color: #a975d6;"'
    + '            type="button"'
    + '            v-on:click="extendReservation()"'
    + '          >'
    + '            <span>Extend</span>'
    + '          </button>'
    + '        </div>'
    + '      </div>'
    + '    </div>'
    + '  </div>'
    + ''
    + '  <loading-modal'
    + '    body="This may take some time..."'
    + '    header="Extending Reservation"'
    + '    ref="loadingModal"'
    + '  ></loading-modal>'
    + '</div>
    + '';
  window.ExtendReservationModal = {
    template: template,
    components: {
      LoadingModal: LoadingModal,
    },
    data: function data() {
      return {
        resName: '',
        timeRange: '60m',
      };
    },
    beforeDestroy: function beforeDestroy() {
      $(this.$refs['modal']).modal('hide');
    },
    computed: {
      validForm: function validForm() {
        return this.resName !== '';
      },
      command: function command() {
        let time = '';

        if (this.timeRange) {
          time = ' -t '.concat(this.timeRange);
        }

        return 'igor extend -r '.concat(this.resName).concat(time);
      },
    },
    methods: {
      show: function show() {
        const res = this.$store.state.selectedReservation;

        if (res) {
          this.resName = res.Name;
        }

        $(this.$refs['modal']).modal('show');
      },
      hide: function hide() {
        $(this.$refs['modal']).modal('hide');
      },
      showLoading: function showLoading() {
        this.$refs['loadingModal'].show();
      },
      hideLoading: function hideLoading() {
        setTimeout(this.$refs['loadingModal'].hide, 500);
      },
      extendReservation: function extendReservation() {
        const _this = this;

        this.hide();
        this.showLoading();
        $.get('run/', {
          run: this.command,
        }, function(data) {
          const response = JSON.parse(data);

          _this.$store.commit('setAlert', response.Message);

          _this.hideLoading();
        });
      },
    },
  };
})();
