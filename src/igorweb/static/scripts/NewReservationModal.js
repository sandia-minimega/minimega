(function() {
  const template = `
    <div id="outer">
      <!-- New reservation modal -->
      <div
        aria-hidden="true"
        aria-labelledby="New Reservation"
        class="modal fade"
        ref="modal"
        role="dialog"
        tabindex="-1"
      >
        <div class="modal-dialog modal-dialog-centered" role="document">
          <div class="modal-content">
            <div class="modal-header m-3">
              <h5 class="modal-title text-center col-12">
                <b v-if="!speculating">New Reservation</b>
                <b v-if="speculating">Available Reservations</b>
              </h5>
              <button
                aria-label="Close"
                class="close"
                data-dismiss="modal"
                style="position: absolute; right: 15px; top: 10px;"
                type="button"
              >
                <span aria-hidden="true">&times;</span>
              </button>
            </div>
            <div class="modal-body m-3">
              <form v-if="!speculating">
                <!-- Reservation name, -r -->
                <div class="form-group">
                  <div
                    class="input-group"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Reservation name"
                  >
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-r</code>
                      </div>
                    </div>
                    <input
                      autofocus
                      class="dash form-control"
                      placeholder="Reservation name"
                      type="text"
                      v-model="name"
                    >
                  </div>
                </div>
                <!-- Switch for (kernel and initrd) or (cobbler profile) -->
                <div
                  aria-label="Use kernel and initrd or Cobbler profile?"
                  class="btn-group"
                  role="group"
                  style="width: 100%; margin-bottom: 0;"
                >
                  <button
                    :class="{active: isKernelInit}"
                    class="modalswitch btn btn-light"
                    style="width: 50%;"
                    type="button"
                    v-on:click="isKernelInit = true"
                  >Use kernel and initrd</button>
                  <button
                    :class="{active: !isKernelInit}"
                    class="modalswitch btn btn-light"
                    style="width: 50%;"
                    type="button"
                    v-on:click="isKernelInit = false"
                  >Use Cobbler profile</button>
                </div>
                <!-- Kernel path, -k, only shows if left side of above switch is active -->
                <div
                  class="form-group"
                  style="margin-bottom: 10px;"
                  v-if="isKernelInit"
                >
                  <div
                    class="input-group"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Location of the kernel the nodes should boot. This kernel will be copied to a separate directory for use."
                  >
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-k</code>
                      </div>
                    </div>
                    <input
                      :class="{'is-valid': kernelPathIsValid, 'is-invalid': !kernelPathIsValid}"
                      class="dash form-control"
                      placeholder="Kernel path"
                      type="text"
                      v-model="kernelPath"
                    >
                    <div
                      class="valid-feedback"
                      v-if="kernelPathIsValid"
                    >Looking good!</div>
                    <div
                      class="invalid-feedback"
                      v-if="!kernelPathIsValid"
                    >Path must be an absolute path to a kernel.</div>
                  </div>
                </div>
                <!-- Initrd path, -i, only shows if left side of above switch is active -->
                <div class="form-group" v-if="isKernelInit">
                  <div
                    class="input-group"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Location of the initrd the nodes should boot. This file will be copied to a separate directory for use."
                  >
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-i</code>
                      </div>
                    </div>
                    <input
                      :class="{'is-valid': initrdPathIsValid, 'is-invalid': !initrdPathIsValid}"
                      class="dash form-control"
                      placeholder="Initrd path"
                      type="text"
                      v-model="initrdPath"
                    >

                    <div
                      class="valid-feedback"
                      v-if="initrdPathIsValid"
                    >Looking good!</div>
                    <div
                      class="invalid-feedback"
                      v-if="!initrdPathIsValid"
                    >Path must be an absolute path to an initial RAM disk.</div>
                  </div>
                  <div>
                    <select v-model="kernelPair">
                      <option disabled value>Choose a kernel pair</option>
                      <option
                        v-bind:value="{kernelPath: item.kernel, initrdPath: item.initrd}"
                        v-for="item in images"
                      >{{ item.name }}</option>
                    </select>
                  </div>
                </div>
                <!-- Cobbler profile, -profile, only shows if right side of above switch is active -->
                <div class="form-group" v-if="!isKernelInit">
                  <div class="input-group">
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-profile</code>
                      </div>
                    </div>
                    <input
                      class="dash form-control"
                      placeholder="Cobbler profile"
                      type="text"
                      v-model="cobblerProfile"
                    >
                  </div>
                </div>
                <!-- Switch for (number of nodes) or (node list) -->
                <div
                  aria-label="Number of nodes or node list?"
                  class="btn-group"
                  role="group"
                  style="width: 100%; margin-bottom: 0;"
                >
                  <button
                    :class="{active: !isNodeList}"
                    class="modalswitch btn btn-light"
                    style="width: 50%;"
                    type="button"
                    v-on:click="isNodeList = false"
                  >Number of nodes</button>
                  <button
                    :class="{active: isNodeList}"
                    class="modalswitch btn btn-light"
                    style="width: 50%;"
                    type="button"
                    v-on:click="isNodeList = true"
                  >Node list</button>
                </div>
                <!-- Number of nodes, -n, only shows if left side of above switch is active -->
                <div class="form-group" v-if="!isNodeList">
                  <div
                    class="input-group"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Number of nodes"
                  >
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-n</code>
                      </div>
                    </div>
                    <input
                      class="dash form-control"
                      min="1"
                      placeholder="Number of nodes"
                      type="number"
                      v-model="numNodes"
                    >
                  </div>
                </div>
                <!-- Node list, -w, only shows if the right side of the above switch is active -->
                <div class="form-group" v-if="isNodeList">
                  <div
                    class="input-group"
                    data-placement="bottom"
                    data-toggle="tooltip"
                    title="Node list, e.g. 34, 57, 158 ..."
                  >
                    <div class="input-group-prepend">
                      <div class="input-group-text">
                        <code>-w</code>
                      </div>
                    </div>
                    <input
                      class="dash form-control"
                      placeholder="Node list"
                      type="text"
                      v-model="nodeList"
                    >
                  </div>
                </div>
                <!-- The rest of the fields are optional -->
                <i class="mb-2">Optional:</i>
                <div
                  class="mb-4"
                  style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;"
                >
                  <!-- Group, -g, optional -->
                  <div class="form-group">
                    <div
                      class="input-group"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="An optional group that can modify this reservation"
                    >
                      <div class="input-group-prepend">
                        <div class="input-group-text">
                          <code style="color: royalblue;">-g</code>
                        </div>
                      </div>
                      <input
                        :class="{'is-valid': group && groupIsValid, 'is-invalid': group && !groupIsValid}"
                        class="dash form-control"
                        placeholder="Group"
                        type="text"
                        v-model="group"
                      >
                      <div
                        class="valid-feedback"
                        v-if="group && groupIsValid"
                      >Looking good!</div>
                      <div
                        class="invalid-feedback"
                        v-if="group && !groupIsValid"
                      >Invalid group name.</div>
                    </div>
                  </div>
                  <!-- Command line arguments, -c, optional -->
                  <div class="form-group">
                    <div
                      class="input-group"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="e.g. console=tty0"
                    >
                      <div class="input-group-prepend">
                        <div class="input-group-text">
                          <code style="color: royalblue;">-c</code>
                        </div>
                      </div>
                      <input
                        class="dash form-control"
                        placeholder="Command line arguments"
                        type="text"
                        v-model="cmdArgs"
                      >
                    </div>
                  </div>
                  <!-- Reservation length, -t, optional, 60m by default -->
                  <div class="form-group">
                    <div
                      class="input-group"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="Time denominations should be specified in days(d), hours(h), and minutes(m), in that order. Unitless numbers are treated as minutes. Days are defined as 24*60 minutes. Example: To make a reservation for 7 days: 7d. To make a reservation for 4 days, 6 hours, 30 minutes: 4d6h30m (default = 60m)."
                    >
                      <div class="input-group-prepend">
                        <div class="input-group-text">
                          <code style="color: royalblue;">-t</code>
                        </div>
                      </div>
                      <input
                        class="dash form-control"
                        placeholder="Reservation length"
                        type="text"
                        v-model="resLength"
                        value="60m"
                      >
                    </div>
                  </div>
                  <!-- After this date, -a, optional, set automatically if Reserve is clicked from Speculate page -->
                  <div class="form-group">
                    <div
                      class="input-group"
                      data-placement="bottom"
                      data-toggle="tooltip"
                      title="Indicates that the reservation should take place on or after the specified time, given in the format '2017-Jan-2-15:04'. Especially useful on Speculate."
                    >
                      <div class="input-group-prepend">
                        <div class="input-group-text">
                          <code style="color: royalblue;">-a</code>
                        </div>
                      </div>
                      <input
                        class="dash form-control"
                        placeholder="After this date"
                        type="text"
                        v-model="afterDate"
                      >
                    </div>
                  </div>
                </div>
              </form>

              <speculate-table
                v-bind:cmd="command"
                v-if="speculating"
                v-on:reserve="reserveSpec($event)"
              ></speculate-table>

              <div class="card commandline" v-if="!speculating">
                <code id="commandline" style="color: seagreen;">{{ command }}</code>
              </div>
            </div>
            <div class="modal-footer m-3">
              <button
                class="modalbtn igorbtn btn btn-secondary mr-auto"
                type="button"
                v-if="speculating"
                v-on:click="speculating = false"
              >Back</button>

              <button
                class="modalbtn igorbtn btn btn-secondary mr-auto cancel"
                data-dismiss="modal"
                type="button"
                v-if="!speculating"
              >Cancel</button>

              <button
                :disabled="!validForm"
                class="modalbtn igorbtn btn btn-primary modalcommand"
                style="background-color: #ff902d; border-color: #ff902d;"
                type="button"
                v-if="!speculating"
                v-on:click="speculating = !speculating"
              >
                <span class>Speculate</span>
              </button>

              <button
                :disabled="!validForm"
                class="modalbtn igorbtn btn btn-primary modalcommand"
                style="background-color: #a975d6; border-color: #a975d6;"
                type="button"
                v-if="!speculating"
                v-on:click="submitReservation()"
              >
                <span>Reserve</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <loading-modal
        body="This may take some time..."
        header="Submitting reservation"
        ref="loadingModal"
      ></loading-modal>
    </div>
  `;

  window.NewReservationModal = {
    template: template,

    components: {
      SpeculateTable,
      LoadingModal,
    },

    data() {
      let latestImage = this.$store.state.recentImages[0];
      if (!latestImage) {
        latestImage = {
          kernelPath: '',
          initrdPath: '',
        };
      }

      return {
        speculating: false,

        name: '',
        kernelPair: '',
        kernelPath: latestImage.kernelPath,
        initrdPath: latestImage.initrdPath,
        cobblerProfile: '',
        numNodes: '',
        nodeList: '',
        group: '',
        cmdArgs: '',
        resLength: '60m',
        afterDate: '',

        isKernelInit: true,
        isNodeList: false,

        serverMessage: '',
        serverSuccess: true,
      };
    },
    watch: {
      kernelPath() {
        this.kernelPair = '';
      },
      initrdPath() {
        this.kernelPair = '';
      },
      kernelPair(value) {
        if (value) {
          this.kernelPath = value.kernelPath;
          this.initrdPath = value.initrdPath;
        }
      },
    },
    computed: {
      groupIsValid() {
        const re = new RegExp('^[_a-z][0-9a-z_-]*\\$?$');
        return this.group.match(re) != null;
      },

      kernelPathIsValid() {
        const re = new RegExp('^(/[^/]*)+[^/]+\\.kernel$');
        return this.kernelPath.match(re) != null;
      },

      initrdPathIsValid() {
        const re = new RegExp('^(/[^/]*)+[^/]+\\.initrd$');
        return this.initrdPath.match(re) != null;
      },
      images() {
        return this.$store.getters.allImages;
      },

      validForm() {
        if (!this.name) {
          return false;
        }

        if (this.group && !this.groupIsValid) {
          return false;
        }

        if (this.isKernelInit) {
          if (!this.kernelPathIsValid || !this.initrdPathIsValid) {
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
          bootFrom = `-k ${this.kernelPath} -i ${this.initrdPath}`;
        }

        let nodes = `-n ${this.numNodes}`;
        if (this.isNodeList) {
          nodes = `-w ${this.nodeList}`;
        }

        let group = '';
        if (this.group) {
          group = ` -g ${this.group}`;
        }

        let args = '';
        if (this.cmdArgs) {
          args = ` -c ${this.cmdArgs}`;
        }

        let after = '';
        if (this.afterDate) {
          after = ` -a ${this.afterDate}`;
        }

        return `igor sub -r ${this.name} ${bootFrom} ${nodes} -t ${this.resLength}${args}${after}${group}`;
      },
    },

    methods: {
      show() {
        const range = this.$store.getters.selectedRange;
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
          if (this.isKernelInit) {
            this.$store.dispatch('saveRecentImage', {
              kernelPath: this.kernelPath,
              initrdPath: this.initrdPath,
            });
          }

          this.showLoading();
          this.hide();

          $.get(
              'run/',
              {run: this.command},
              (data) => {
                const response = JSON.parse(data);
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
