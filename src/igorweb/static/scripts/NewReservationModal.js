(function() {
  const template = `
      <div id="outer">
      <!-- New reservation modal -->
      <div class="modal fade mdl" tabindex="-1" role="dialog" aria-labelledby="New Reservation" aria-hidden="true" ref="modal">
        <div class="modal-dialog modal-dialog-centered mdl" role="document">
          <div class="modal-content mdl">
            <div class="modal-header m-3 mdl">
              <h5 class="modal-title text-center col-12 mdl" id="modaltitle">
                <b class="mdl" v-if="!speculating">New Reservation</b>
                <b class="mdl" v-if="speculating">Available Reservations</b>
              </h5>
              <button type="button" class="close mdl" data-dismiss="modal" aria-label="Close" style="position: absolute; right: 15px; top: 10px;">
                <span class="mdl" aria-hidden="true">&times;</span>
              </button>
            </div>
            <div class="modal-body m-3 mdl">
              <form class="mdl" v-if="!speculating">
                <!-- Reservation name, -r -->
                <div class="form-group mdl">
                  <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Reservation name">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="dashrcode" class="mdl">-r</code></div>
                    </div>
                    <input v-model="name" type="text" class="dash form-control mdl" placeholder="Reservation name" autofocus>
                  </div>
                </div>
                <!-- Switch for (kernel and initrd) or (cobbler profile) -->
                <div class="mdl btn-group" role="group" aria-label="Use kernel and initrd or Cobbler profile?" style="width: 100%; margin-bottom: 0;">
                  <button type="button" class="modalswitch btn btn-light mdl" :class="{active: isKernelInit}" style="width: 50%;" v-on:click="isKernelInit = true">Use kernel and initrd</button>
                  <button type="button" class="modalswitch btn btn-light mdl" :class="{active: !isKernelInit}" style="width: 50%;" v-on:click="isKernelInit = false">Use Cobbler profile</button>
                </div>
                <!-- Kernel path, -k, only shows if left side of above switch is active -->
                <div v-if="isKernelInit" class="form-group switchki mdl" style="margin-bottom: 10px;">
                  <div id="dashkparent" class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Location of the kernel the nodes should boot. This kernel will be copied to a separate directory for use.">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl">
                        <code id="dashkcode" class="mdl">-k</code>
                      </div>
                    </div>
                    <input v-model="kernelPath" type="text" class="dash form-control mdl" placeholder="Kernel path">
                  </div>
                </div>
                <!-- Initrd path, -i, only shows if left side of above switch is active -->
                <div v-if="isKernelInit" class="form-group switchki mdl">
                  <div id="dashiparent" class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Location of the initrd the nodes should boot. This file will be copied to a separate directory for use.">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="dashicode" class="mdl">-i</code></div>
                    </div>
                    <input v-model="initrdPath" type="text" class="dash form-control mdl" placeholder="Initrd path">
                  </div>
                  <div>
                  <select v-model="kernelpair">
                  <option disabled value="">Please select one</option>
                  <option v-for="item in IMAGES" :value="item.name">{{ item.name }}</option>
                  </option>
                  </select>
                </div>
                </div>
                <!-- Cobbler profile, -profile, only shows if right side of above switch is active -->
                <div v-if="!isKernelInit" class="form-group switchcobbler mdl">
                  <div id="dashpparent" class="input-group mdl">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="dashpcode" class="mdl">-profile</code></div>
                    </div>
                    <input v-model="cobblerProfile" type="text" class="dash form-control mdl" placeholder="Cobbler profile">
                  </div>
                </div>
                <!-- Switch for (number of nodes) or (node list) -->
                <div class="mdl btn-group" role="group" aria-label="Number of nodes or node list?" style="width: 100%; margin-bottom: 0;">
                  <button type="button" class="modalswitch btn btn-light mdl" style="width: 50%;" :class="{active: !isNodeList}" v-on:click="isNodeList = false">Number of nodes</button>
                  <button type="button" class="modalswitch btn btn-light mdl" style="width: 50%;" :class="{active: isNodeList}" v-on:click="isNodeList = true">Node list</button>
                </div>
                <!-- Number of nodes, -n, only shows if left side of above switch is active -->
                <div v-if="!isNodeList" class="form-group mdl switchnumnodes">
                  <div id="dashnparent" class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Number of nodes">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="dashncode" class="mdl">-n</code></div>
                    </div>
                    <input v-model="numNodes" type="number" class="dash form-control mdl" min="1" placeholder="Number of nodes">
                  </div>
                </div>
                <!-- Node list, -w, only shows if the right side of the above switch is active -->
                <div v-if="isNodeList" class="form-group mdl switchnodelist">
                  <div id="dashwparent" class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Node list, e.g. 34, 57, 158 ...">
                    <div class="input-group-prepend mdl">
                      <div class="input-group-text mdl"><code id="dashwcode" class="mdl">-w</code></div>
                    </div>
                    <input v-model="nodeList" type="text" class="dash form-control mdl" placeholder="Node list">
                  </div>
                </div>
                <!-- The rest of the fields are optional -->
                <i class="mb-2 mdl">Optional:</i>
                <div class="mb-4 mdl" style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;">
                  <!-- Command line arguments, -c, optional -->
                  <div class="form-group mdl">
                    <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="e.g. console=tty0">
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl"><code id="dashccode" class="mdl" style="color: royalblue;">-c</code></div>
                      </div>
                      <input v-model="cmdArgs" type="text" class="dash form-control mdl" placeholder="Command line arguments">
                    </div>
                  </div>
                  <!-- Reservation length, -t, optional, 60m by default -->
                  <div class="form-group mdl">
                    <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Time denominations should be specified in days(d), hours(h), and minutes(m), in that order. Unitless numbers are treated as minutes. Days are defined as 24*60 minutes. Example: To make a reservation for 7 days: 7d. To make a reservation for 4 days, 6 hours, 30 minutes: 4d6h30m (default = 60m).">
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl"><code id="dashtkcode" class="mdl" style="color: royalblue;">-t</code></div>
                      </div>
                      <input v-model="resLength" value="60m" type="text" class="dash form-control mdl" placeholder="Reservation length">
                    </div>
                  </div>
                  <!-- After this date, -a, optional, set automatically if Reserve is clicked from Speculate page -->
                  <div class="form-group mdl">
                    <div class="input-group mdl" data-toggle="tooltip" data-placement="bottom" title="Indicates that the reservation should take place on or after the specified time, given in the format '2017-Jan-2-15:04'. Especially useful on Speculate.">
                      <div class="input-group-prepend mdl">
                        <div class="input-group-text mdl"><code id="dashacode" class="mdl" style="color: royalblue;">-a</code></div>
                      </div>
                      <input v-model="afterDate" type="text" class="dash form-control mdl" placeholder="After this date">
                    </div>
                  </div>
                </div>
              </form>

              <speculate-table
                v-if="speculating"
                v-bind:cmd="command"
                v-on:reserve="reserveSpec($event)">
              </speculate-table>

              <div class="card commandline mdl" v-if="!speculating">
                <code id="commandline" class="mdl" style="color: seagreen;">
                  {{ command }}
                </code>
              </div>
            </div>
            <div class="modal-footer m-3 mdl">
              <button
                type="button"
                class="modalbtn igorbtn btn btn-secondary mr-auto mdl"
                v-on:click="speculating = false"
                v-if="speculating">
                  Back
              </button>

              <button
                type="button"
                class="modalbtn igorbtn btn btn-secondary mr-auto mdl cancel"
                data-dismiss="modal"
                v-if="!speculating">
                  Cancel
              </button>

              <button
                type="button"
                style="background-color: #ff902d; border-color: #ff902d;"
                class="modalbtn newresmodalgobtn igorbtn btn btn-primary mdl modalcommand speculate"
                :disabled="!validForm"
                v-on:click="speculating = !speculating"
                v-if="!speculating">
                  <span class="mdl mdlcmdtext speculate">Speculate</span>
              </button>

              <button
                type="button"
                style="background-color: #a975d6; border-color: #a975d6;"
                class="modalbtn newresmodalgobtn igorbtn btn btn-primary mdl modalcommand"
                :disabled="!validForm"
                v-on:click="submitReservation()"
                v-if="!speculating">
                  <span class="mdl mdlcmdtext">Reserve</span>
              </button>
            </div>
          </div>
        </div>
      </div>

        <loading-modal
          ref="loadingModal"
          header="Submitting reservation"
          body="This may take some time..."
        >
        </loading-modal>
      </div>
    `;

  window.NewReservationModal = {
    template: template,

    components: {
      SpeculateTable,
      LoadingModal,
    },

    data() {
      return {
        speculating: false,

        name: '',
        kernelPath: '',
        initrdPath: '',
        cobblerProfile: '',
        numNodes: '',
        nodeList: '',
        cmdArgs: '',
        resLength: '60m',
        afterDate: '',
        kernelpair: '',
        isKernelInit: true,
        isNodeList: false,
        selected: null,
        serverMessage: '',
        serverSuccess: true,
      };
    },

    computed: {
      validForm() {
        if (!this.name) {
          return false;
        }

        if (this.isKernelInit) {
          if ((!this.kernelPath || !this.initrdPath) && !this.kernelpair) {
            return false;
          }
        } else {
          if (!this.cobblerProfile) {
            return false;
          }
        }

        if (this.isNodeList) {
          if (!this.nodeList) {
            return false;
          }
        } else {
          if (!this.numNodes) {
            return false;
          }
        }

        return true;
      },

      command() {
        let bootFrom = `-profile ${this.cobblerProfile}`;
        if (this.isKernelInit) {
          if (this.kernelpair && !(this.kernelPath && this.initrdPath)){
            bootFrom = `-k ${IMAGEPATH}${this.kernelpair}.kernel -i ${IMAGEPATH}${this.kernelpair}.initrd`;
          }else {bootFrom = `-k ${this.kernelPath} -i ${this.initrdPath}`;}
        }

        let nodes = `-n ${this.numNodes}`;
        if (this.isNodeList) {
          nodes = `-w ${this.nodeList}`;
        }

        let args = '';
        if (this.cmdArgs) {
          args = ` -c ${this.cmdArgs}`;
        }

        let after = '';
        if (this.afterDate) {
          after = ` -a ${this.afterDate}`;
        }

        return `igor sub -r ${this.name} ${bootFrom} ${nodes} -t ${this.resLength}${args}${after}`;
      },
    },

    methods: {
      show() {
        let range = this.$store.getters.selectedRange;
        if (range != '') {
          this.numNodes = this.$store.state.selectedNodes.length;
          this.nodeList = range;
          this.isNodeList = true;
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

      reserveSpec(formattedStart) {
        this.speculating = false;
        this.afterDate = formattedStart;
      },

      submitReservation() {
        if (this.validForm) {
          this.showLoading();
          this.hide();

          $.get(
            'run/',
            {run: this.command},
            (data) => {
              let response = JSON.parse(data);
              this.$store.commit('updateReservations', response.Extra);
              this.$store.commit('setAlert', `Reservation ${this.name}: ${response.Message}`);
              this.hideLoading();
            }
          );
        }
      },      
    },
  };
})();