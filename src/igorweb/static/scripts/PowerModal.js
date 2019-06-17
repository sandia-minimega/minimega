(function() {
  const template = `
    <div>
      <!-- Power-control modal -->
      <div class="modal fade mdl" id="powermodal" tabindex="-1" role="dialog" aria-labelledby="Power-control" aria-hidden="true" ref="modal">
        <div class="modal-dialog modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl">
              <h5 class="modal-title text-center col-12 mdl" id="pmodaltitle"><b class="mdl">Power Control</b></h5>
              <button type="button" class="close mdl" data-dismiss="modal" aria-label="Close" style="position: absolute; right: 15px; top: 10px;">
                <span class="mdl" aria-hidden="true">&times;</span>
              </button>
            </div>
            <div class="modal-body m-3 mdl">
              <!-- Form with all of the fields -->
              <form class="mdl">
                <!-- Switch between using a reservation or node list -->
                <div class="mdl btn-group" role="group" aria-label="By reservation or node list?" style="width: 100%; margin-bottom: 0;">
                  <button id="pmodalres" type="button" class="modalswitch btn btn-light mdl" :class="{active: byReservation}" style="width: 50%;" v-on:click="byReservation=true">By Reservation</button>
                  <button id="pmodalnodelist" type="button" class="modalswitch btn btn-light mdl" :class="{active: !byReservation}" style="width: 50%;" v-on:click="byReservation=false">By Node List</button>
                </div>
                <!-- Reservation name, -r, only shows if left side of above switch is active -->
                <div id="pdashrfg" class="form-group mdl" v-if="byReservation">
                  <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Reservation name">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="pdashrcode" class="mdl">-r</code></div>
                    </div>
                    <input id="pdashr" type="text" class="pdash form-control mdl" v-model="resName" placeholder="Reservation name" autofocus>
                  </div>
                </div>
                <!-- Node list, -n, only shows if right side of above switch is active -->
                <div id="pdashnfg" class="form-group mdl nodelistoption2" v-if="!byReservation">
                  <div id="pdashnparent" class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Node list, e.g. 34, 57, 158 ...">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="pdashncode" class="mdl">-n</code></div>
                    </div>
                    <input id="pdashn" type="text" class="pdash form-control mdl" v-model="nodeRange" placeholder="Node list">
                  </div>
                </div>
              </form>
              <!-- Command box, updates command text as user constructs it from filling fields.
                   Shows exactly what will be run on igor -->
              <div class="card commandline mdl"><code id="pcommandline" class="mdl" style="color: seagreen;">{{ command }}</code></div>
            </div>
            <!-- Buttons at bottom of modal -->
            <div class="modal-footer m-3 mdl">
              <!-- Cancel, exits modal, only shows on main reservation page -->
              <button type="button" class="modalbtn igorbtn btn btn-secondary mr-auto mdl cancel" data-dismiss="modal">Cancel</button>
              <!-- On, submits a igor power on command to the server -->
              <button type="button" style="background-color: mediumseagreen; border-color: mediumseagreen;" class="modalbtn powermodalgobtn igorbtn btn btn-primary mdl modalcommand" id="on" v-on:click="submitPower('on')" :disabled="!validForm"><span class="mdl mdlcmdtext">On</span></button>
              <!-- Off, submits a igor power off command to the server -->
              <button type="button" style="background-color: lightcoral; border-color: lightcoral;" class="modalbtn powermodalgobtn igorbtn btn btn-primary mdl modalcommand" id="off" v-on:click="submitPower('off')" :disabled="!validForm"><span class="mdl mdlcmdtext">Off</span><div style="visibility: hidden" class="mdl loader"></div></button>
              <!-- control, submits a igor power control command to the server -->
              <button type="button" class="modalbtn powermodalgobtn igorbtn btn btn-primary mdl modalcommand" id="control" v-on:click="submitPower('cycle')" :disabled="!validForm"><span class="mdl mdlcmdtext">Cycle</span><div style="visibility: hidden" class="mdl loader"></div></button>
            </div>
          </div>
        </div>
      </div>

      <loading-modal
        ref="loadingModal"
        header="Issuing Power Command"
        body="This may take some time..."
      >
      </loading-modal>
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
