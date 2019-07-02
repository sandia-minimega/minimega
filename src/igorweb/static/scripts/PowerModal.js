'use strict';

(function() {
  const template = ''
    + '<div>'
    + '  <!-- Power-control modal -->'
    + '  <div'
    + '    aria-hidden="true"'
    + '    aria-labelledby="Power-control"'
    + '    class="modal fade"'
    + '    ref="modal"'
    + '    role="dialog"'
    + '    tabindex="-1"'
    + '  >'
    + '    <div class="modal-dialog modal-dialog-centered" role="document">'
    + '      <div class="modal-content">'
    + '        <div class="modal-header m-3">'
    + '          <h5 class="modal-title text-center col-12">'
    + '            <b>Power Control</b>'
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
    + '        <div class="modal-body m-3">'
    + '          <!-- Form with all of the fields -->'
    + '          <form>'
    + '            <!-- Switch between using a reservation or node list -->'
    + '            <div'
    + '              aria-label="By reservation or node list?"'
    + '              class="btn-group"'
    + '              role="group"'
    + '              style="width: 100%; margin-bottom: 0;"'
    + '            >'
    + '              <button'
    + '                :class="{active: byReservation}"'
    + '                class="modalswitch btn btn-light"'
    + '                style="width: 50%;"'
    + '                type="button"'
    + '                v-on:click="byReservation=true"'
    + '              >By Reservation</button>'
    + '              <button'
    + '                :class="{active: !byReservation}"'
    + '                class="modalswitch btn btn-light"'
    + '                style="width: 50%;"'
    + '                type="button"'
    + '                v-on:click="byReservation=false"'
    + '              >By Node List</button>'
    + '            </div>'
    + '            <!-- Reservation name, -r, only shows if left side of above switch is active -->'
    + '            <div class="form-group" id="rfg" v-if="byReservation">'
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
    + '            <!-- Node list, -n, only shows if right side of above switch is active -->'
    + '            <div class="form-group" v-if="!byReservation">'
    + '              <div'
    + '                class="input-group"'
    + '                data-placement="bottom"'
    + '                data-toggle="tooltip"'
    + '                id="p"'
    + '                title="Node list, e.g. 34, 57, 158 ..."'
    + '              >'
    + '                <div class="input-group-prepend">'
    + '                  <div class="input-group-text">'
    + '                    <code>-n</code>'
    + '                  </div>'
    + '                </div>'
    + '                <input'
    + '                  class="form-control"'
    + '                  id="n"'
    + '                  placeholder="Node list"'
    + '                  type="text"'
    + '                  v-model="nodeRange"'
    + '                >'
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
    + '          <!-- On, submits a igor power on command to the server -->'
    + '          <button'
    + '            :disabled="!validForm"'
    + '            class="modalbtn gobtn igorbtn btn btn-primary modalcommand"'
    + '            id="on"'
    + '            style="background-color: mediumseagreen; border-color: mediumseagreen;"'
    + '            type="button"'
    + '            v-on:click="submitPower(\'on\')"'
    + '          >'
    + '            <span>On</span>'
    + '          </button>'
    + '          <!-- Off, submits a igor power off command to the server -->'
    + '          <button'
    + '            :disabled="!validForm"'
    + '            class="modalbtn gobtn igorbtn btn btn-primary modalcommand"'
    + '            id="off"'
    + '            style="background-color: lightcoral; border-color: lightcoral;"'
    + '            type="button"'
    + '            v-on:click="submitPower(\'off\')"'
    + '          >'
    + '            <span>Off</span>'
    + '            <div class="loader" style="visibility: hidden"></div>'
    + '          </button>'
    + '          <!-- control, submits a igor power control command to the server -->'
    + '          <button'
    + '            :disabled="!validForm"'
    + '            class="modalbtn gobtn igorbtn btn btn-primary modalcommand"'
    + '            id="control"'
    + '            type="button"'
    + '            v-on:click="submitPower(\'cycle\')"'
    + '          >'
    + '            <span>Cycle</span>'
    + '            <div class="loader" style="visibility: hidden"></div>'
    + '          </button>'
    + '        </div>'
    + '      </div>'
    + '    </div>'
    + '  </div>'
    + ''
    + '  <loading-modal'
    + '    body="This may take some time..."'
    + '    header="Issuing Power Command"'
    + '    ref="loadingModal"'
    + '  ></loading-modal>'
    + '</div>'
    + '';
  window.PowerModal = {
    template: template,
    components: {
      LoadingModal: LoadingModal,
    },
    data: function data() {
      return {
        resName: '',
        nodeRange: '',
        byReservation: true,
      };
    },
    beforeDestroy: function beforeDestroy() {
      $(this.$refs['modal']).modal('hide');
    },
    computed: {
      validForm: function validForm() {
        if (this.byReservation) {
          return this.resName !== '';
        }

        return this.nodeRange !== '';
      },
      command: function command() {
        if (this.byReservation) {
          return 'igor power -r '.concat(this.resName);
        }

        return 'igor power -n '.concat(this.nodeRange);
      },
    },
    methods: {
      show: function show() {
        const range = this.$store.getters.selectedRange;

        if (range) {
          this.nodeRange = range;
          this.byReservation = false;
        }

        const res = this.$store.state.selectedReservation;

        if (res) {
          this.resName = res.Name;
          this.byReservation = true;
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
      submitPower: function submitPower(cmd) {
        const _this = this;

        this.hide();
        this.showLoading();
        $.get('run/', {
          run: ''.concat(this.command, ' ').concat(cmd),
        }, function(data) {
          const response = JSON.parse(data);

          _this.$store.commit('setAlert', response.Message);

          _this.hideLoading();
        });
      },
    },
  };
})();
