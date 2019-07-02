(function() {
  const template = `
    <div>
      <!-- Power-control modal -->
      <div
        aria-hidden="true"
        aria-labelledby="Power-control"
        class="modal fade mdl"
        id="powermodal"
        ref="modal"
        role="dialog"
        tabindex="-1"
      >
        <div class="modal-dialog modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl">
              <h5 class="modal-title text-center col-12 mdl" id="pmodaltitle">
                <b class="mdl">Power Control</b>
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
            <div class="modal-body m-3 mdl">
              <!-- Form with all of the fields -->
              <form class="mdl">
                <!-- Switch between using a reservation or node list -->
                <div
                  aria-label="By reservation or node list?"
                  class="mdl btn-group"
                  role="group"
                  style="width: 100%; margin-bottom: 0;"
                >
                  <button
                    :class="{active: byReservation}"
                    class="modalswitch btn btn-light mdl"
                    id="pmodalres"
                    style="width: 50%;"
                    type="button"
                    v-on:click="byReservation=true"
                  >By Reservation</button>
                  <button
                    :class="{active: !byReservation}"
                    class="modalswitch btn btn-light mdl"
                    id="pmodalnodelist"
                    style="width: 50%;"
                    type="button"
                    v-on:click="byReservation=false"
                  >By Node List</button>
                </div>
                <!-- Reservation name, -r, only shows if left side of above switch is active -->
                <div class="form-group mdl" id="pdashrfg" v-if="byReservation">
                  <div
                    class="input-group mdl"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Reservation name"
                  >
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code class="mdl" id="pdashrcode">-r</code>
                      </div>
                    </div>
                    <input
                      autofocus
                      class="pdash form-control mdl"
                      id="pdashr"
                      placeholder="Reservation name"
                      type="text"
                      v-model="resName"
                    >
                  </div>
                </div>
                <!-- Node list, -n, only shows if right side of above switch is active -->
                <div
                  class="form-group mdl nodelistoption2"
                  id="pdashnfg"
                  v-if="!byReservation"
                >
                  <div
                    class="input-group mdl"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    id="pdashnparent"
                    title="Node list, e.g. 34, 57, 158 ..."
                  >
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code class="mdl" id="pdashncode">-n</code>
                      </div>
                    </div>
                    <input
                      class="pdash form-control mdl"
                      id="pdashn"
                      placeholder="Node list"
                      type="text"
                      v-model="nodeRange"
                    >
                  </div>
                </div>
              </form>
              <!-- Command box, updates command text as user constructs it from filling fields.
              Shows exactly what will be run on igor-->
              <div class="card commandline mdl">
                <code
                  class="mdl"
                  id="pcommandline"
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
              <!-- On, submits a igor power on command to the server -->
              <button
                :disabled="!validForm"
                class="modalbtn powermodalgobtn igorbtn btn btn-primary mdl modalcommand"
                id="on"
                style="background-color: mediumseagreen; border-color: mediumseagreen;"
                type="button"
                v-on:click="submitPower('on')"
              >
                <span class="mdl mdlcmdtext">On</span>
              </button>
              <!-- Off, submits a igor power off command to the server -->
              <button
                :disabled="!validForm"
                class="modalbtn powermodalgobtn igorbtn btn btn-primary mdl modalcommand"
                id="off"
                style="background-color: lightcoral; border-color: lightcoral;"
                type="button"
                v-on:click="submitPower('off')"
              >
                <span class="mdl mdlcmdtext">Off</span>
                <div class="mdl loader" style="visibility: hidden"></div>
              </button>
              <!-- control, submits a igor power control command to the server -->
              <button
                :disabled="!validForm"
                class="modalbtn powermodalgobtn igorbtn btn btn-primary mdl modalcommand"
                id="control"
                type="button"
                v-on:click="submitPower('cycle')"
              >
                <span class="mdl mdlcmdtext">Cycle</span>
                <div class="mdl loader" style="visibility: hidden"></div>
              </button>
            </div>
          </div>
        </div>
      </div>

      <loading-modal
        body="This may take some time..."
        header="Issuing Power Command"
        ref="loadingModal"
      ></loading-modal>
    </div>
  `;

  window.PowerModal = {
    template: template,

    components: {
      LoadingModal,
    },

    data() {
      return {
        resName: '',
        nodeRange: '',

        byReservation: true,
      };
    },

    beforeDestroy() {
      $(this.$refs['modal']).modal('hide');
    },

    computed: {
      validForm() {
        if (this.byReservation) {
          return this.resName !== '';
        }

        return this.nodeRange !== '';
      },

      command() {
        if (this.byReservation) {
          return `igor power -r ${this.resName}`;
        }
        return `igor power -n ${this.nodeRange}`;
      },
    },

    methods: {
      show() {
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

      hide() {
        $(this.$refs['modal']).modal('hide');
      },

      showLoading() {
        this.$refs['loadingModal'].show();
      },

      hideLoading() {
        setTimeout(this.$refs['loadingModal'].hide, 500);
      },

      submitPower(cmd) {
        this.hide();
        this.showLoading();

        $.get(
            'run/',
            {run: `${this.command} ${cmd}`},
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
