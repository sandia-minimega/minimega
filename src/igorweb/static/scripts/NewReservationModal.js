'use strict';

(function() {
  const template = ''
    + '<div id="outer">'
    + '  <!-- New reservation modal -->'
    + '  <div'
    + '    aria-hidden="true"'
    + '    aria-labelledby="New Reservation"'
    + '    class="modal fade mdl"'
    + '    ref="modal"'
    + '    role="dialog"'
    + '    tabindex="-1"'
    + '  >'
    + '    <div class="modal-dialog modal-dialog-centered mdl" role="document">'
    + '      <div class="modal-content mdl">'
    + '        <div class="modal-header m-3 mdl">'
    + '          <h5 class="modal-title text-center col-12 mdl" id="modaltitle">'
    + '            <b class="mdl" v-if="!speculating">New Reservation</b>'
    + '            <b class="mdl" v-if="speculating">Available Reservations</b>'
    + '          </h5>'
    + '          <button'
    + '            aria-label="Close"'
    + '            class="close mdl"'
    + '            data-dismiss="modal"'
    + '            style="position: absolute; right: 15px; top: 10px;"'
    + '            type="button"'
    + '          >'
    + '            <span aria-hidden="true" class="mdl">&times;</span>'
    + '          </button>'
    + '        </div>'
    + '        <div class="modal-body m-3 mdl">'
    + '          <form class="mdl" v-if="!speculating">'
    + '            <!-- Reservation name, -r -->'
    + '            <div class="form-group mdl">'
    + '              <div'
    + '                class="input-group mdl"'
    + '                data-placement="bottom"'
    + '                data-toggle="tooltip"'
    + '                title="Reservation name"'
    + '              >'
    + '                <div class="input-group-prepend mdl">'
    + '                  <div class="input-group-text mdl">'
    + '                    <code class="mdl" id="dashrcode">-r</code>'
    + '                  </div>'
    + '                </div>'
    + '                <input'
    + '                  autofocus'
    + '                  class="dash form-control mdl"'
    + '                  placeholder="Reservation name"'
    + '                  type="text"'
    + '                  v-model="name"'
    + '                >'
    + '              </div>'
    + '            </div>'
    + '            <!-- Switch for (kernel and initrd) or (cobbler profile) -->'
    + '            <div'
    + '              aria-label="Use kernel and initrd or Cobbler profile?"'
    + '              class="mdl btn-group"'
    + '              role="group"'
    + '              style="width: 100%; margin-bottom: 0;"'
    + '            >'
    + '              <button'
    + '                :class="{active: isKernelInit}"'
    + '                class="modalswitch btn btn-light mdl"'
    + '                style="width: 50%;"'
    + '                type="button"'
    + '                v-on:click="isKernelInit = true"'
    + '              >Use kernel and initrd</button>'
    + '              <button'
    + '                :class="{active: !isKernelInit}"'
    + '                class="modalswitch btn btn-light mdl"'
    + '                style="width: 50%;"'
    + '                type="button"'
    + '                v-on:click="isKernelInit = false"'
    + '              >Use Cobbler profile</button>'
    + '            </div>'
    + '            <!-- Kernel path, -k, only shows if left side of above switch is active -->'
    + '            <div'
    + '              class="form-group switchki mdl"'
    + '              style="margin-bottom: 10px;"'
    + '              v-if="isKernelInit"'
    + '            >'
    + '              <div'
    + '                class="input-group mdl"'
    + '                data-placement="bottom"'
    + '                data-toggle="tooltip"'
    + '                id="dashkparent"'
    + '                title="Location of the kernel the nodes should boot. This kernel will be copied to a separate directory for use."'
    + '              >'
    + '                <div class="input-group-prepend mdl">'
    + '                  <div class="input-group-text mdl">'
    + '                    <code class="mdl" id="dashkcode">-k</code>'
    + '                  </div>'
    + '                </div>'
    + '                <input'
    + '                  class="dash form-control mdl"'
    + '                  placeholder="Kernel path"'
    + '                  type="text"'
    + '                  v-model="kernelPath"'
    + '                >'
    + '              </div>'
    + '            </div>'
    + '            <!-- Initrd path, -i, only shows if left side of above switch is active -->'
    + '            <div class="form-group switchki mdl" v-if="isKernelInit">'
    + '              <div'
    + '                class="input-group mdl"'
    + '                data-placement="bottom"'
    + '                data-toggle="tooltip"'
    + '                id="dashiparent"'
    + '                title="Location of the initrd the nodes should boot. This file will be copied to a separate directory for use."'
    + '              >'
    + '                <div class="input-group-prepend mdl">'
    + '                  <div class="input-group-text mdl">'
    + '                    <code class="mdl" id="dashicode">-i</code>'
    + '                  </div>'
    + '                </div>'
    + '                <input'
    + '                  class="dash form-control mdl"'
    + '                  placeholder="Initrd path"'
    + '                  type="text"'
    + '                  v-model="initrdPath"'
    + '                >'
    + '              </div>'
    + '              <div>'
    + '                <select v-model="kernelpair">'
    + '                  <option disabled value>Please select one</option>'
    + '                  <option'
    + '                    :value="item.name"'
    + '                    v-for="item in IMAGES"'
    + '                  >{{ item.name }}</option>'
    + '                </select>'
    + '              </div>'
    + '            </div>'
    + '            <!-- Cobbler profile, -profile, only shows if right side of above switch is active -->'
    + '            <div class="form-group switchcobbler mdl" v-if="!isKernelInit">'
    + '              <div class="input-group mdl" id="dashpparent">'
    + '                <div class="input-group-prepend mdl">'
    + '                  <div class="input-group-text mdl">'
    + '                    <code class="mdl" id="dashpcode">-profile</code>'
    + '                  </div>'
    + '                </div>'
    + '                <input'
    + '                  class="dash form-control mdl"'
    + '                  placeholder="Cobbler profile"'
    + '                  type="text"'
    + '                  v-model="cobblerProfile"'
    + '                >'
    + '              </div>'
    + '            </div>'
    + '            <!-- Switch for (number of nodes) or (node list) -->'
    + '            <div'
    + '              aria-label="Number of nodes or node list?"'
    + '              class="mdl btn-group"'
    + '              role="group"'
    + '              style="width: 100%; margin-bottom: 0;"'
    + '            >'
    + '              <button'
    + '                :class="{active: !isNodeList}"'
    + '                class="modalswitch btn btn-light mdl"'
    + '                style="width: 50%;"'
    + '                type="button"'
    + '                v-on:click="isNodeList = false"'
    + '              >Number of nodes</button>'
    + '              <button'
    + '                :class="{active: isNodeList}"'
    + '                class="modalswitch btn btn-light mdl"'
    + '                style="width: 50%;"'
    + '                type="button"'
    + '                v-on:click="isNodeList = true"'
    + '              >Node list</button>'
    + '            </div>'
    + '            <!-- Number of nodes, -n, only shows if left side of above switch is active -->'
    + '            <div class="form-group mdl switchnumnodes" v-if="!isNodeList">'
    + '              <div'
    + '                class="input-group mdl"'
    + '                data-placement="bottom"'
    + '                data-toggle="tooltip"'
    + '                id="dashnparent"'
    + '                title="Number of nodes"'
    + '              >'
    + '                <div class="input-group-prepend mdl">'
    + '                  <div class="input-group-text mdl">'
    + '                    <code class="mdl" id="dashncode">-n</code>'
    + '                  </div>'
    + '                </div>'
    + '                <input'
    + '                  class="dash form-control mdl"'
    + '                  min="1"'
    + '                  placeholder="Number of nodes"'
    + '                  type="number"'
    + '                  v-model="numNodes"'
    + '                >'
    + '              </div>'
    + '            </div>'
    + '            <!-- Node list, -w, only shows if the right side of the above switch is active -->'
    + '            <div class="form-group mdl switchnodelist" v-if="isNodeList">'
    + '              <div'
    + '                class="input-group mdl"'
    + '                data-placement="bottom"'
    + '                data-toggle="tooltip"'
    + '                id="dashwparent"'
    + '                title="Node list, e.g. 34, 57, 158 ..."'
    + '              >'
    + '                <div class="input-group-prepend mdl">'
    + '                  <div class="input-group-text mdl">'
    + '                    <code class="mdl" id="dashwcode">-w</code>'
    + '                  </div>'
    + '                </div>'
    + '                <input'
    + '                  class="dash form-control mdl"'
    + '                  placeholder="Node list"'
    + '                  type="text"'
    + '                  v-model="nodeList"'
    + '                >'
    + '              </div>'
    + '            </div>'
    + '            <!-- The rest of the fields are optional -->'
    + '            <i class="mb-2 mdl">Optional:</i>'
    + '            <div'
    + '              class="mb-4 mdl"'
    + '              style="border-top: 1px solid #e9ecef; border-bottom: 1px solid #e9ecef; padding-top: 5px;"'
    + '            >'
    + '              <!-- Command line arguments, -c, optional -->'
    + '              <div class="form-group mdl">'
    + '                <div'
    + '                  class="input-group mdl"'
    + '                  data-placement="bottom"'
    + '                  data-toggle="tooltip"'
    + '                  title="e.g. console=tty0"'
    + '                >'
    + '                  <div class="input-group-prepend mdl">'
    + '                    <div class="input-group-text mdl">'
    + '                      <code'
    + '                        class="mdl"'
    + '                        id="dashccode"'
    + '                        style="color: royalblue;"'
    + '                      >-c</code>'
    + '                    </div>'
    + '                  </div>'
    + '                  <input'
    + '                    class="dash form-control mdl"'
    + '                    placeholder="Command line arguments"'
    + '                    type="text"'
    + '                    v-model="cmdArgs"'
    + '                  >'
    + '                </div>'
    + '              </div>'
    + '              <!-- Reservation length, -t, optional, 60m by default -->'
    + '              <div class="form-group mdl">'
    + '                <div'
    + '                  class="input-group mdl"'
    + '                  data-placement="bottom"'
    + '                  data-toggle="tooltip"'
    + '                  title="Time denominations should be specified in days(d), hours(h), and minutes(m), in that order. Unitless numbers are treated as minutes. Days are defined as 24*60 minutes. Example: To make a reservation for 7 days: 7d. To make a reservation for 4 days, 6 hours, 30 minutes: 4d6h30m (default = 60m)."'
    + '                >'
    + '                  <div class="input-group-prepend mdl">'
    + '                    <div class="input-group-text mdl">'
    + '                      <code'
    + '                        class="mdl"'
    + '                        id="dashtkcode"'
    + '                        style="color: royalblue;"'
    + '                      >-t</code>'
    + '                    </div>'
    + '                  </div>'
    + '                  <input'
    + '                    class="dash form-control mdl"'
    + '                    placeholder="Reservation length"'
    + '                    type="text"'
    + '                    v-model="resLength"'
    + '                    value="60m"'
    + '                  >'
    + '                </div>'
    + '              </div>'
    + '              <!-- After this date, -a, optional, set automatically if Reserve is clicked from Speculate page -->'
    + '              <div class="form-group mdl">'
    + '                <div'
    + '                  class="input-group mdl"'
    + '                  data-placement="bottom"'
    + '                  data-toggle="tooltip"'
    + '                  title="Indicates that the reservation should take place on or after the specified time, given in the format \'2017-Jan-2-15:04\'. Especially useful on Speculate."'
    + '                >'
    + '                  <div class="input-group-prepend mdl">'
    + '                    <div class="input-group-text mdl">'
    + '                      <code'
    + '                        class="mdl"'
    + '                        id="dashacode"'
    + '                        style="color: royalblue;"'
    + '                      >-a</code>'
    + '                    </div>'
    + '                  </div>'
    + '                  <input'
    + '                    class="dash form-control mdl"'
    + '                    placeholder="After this date"'
    + '                    type="text"'
    + '                    v-model="afterDate"'
    + '                  >'
    + '                </div>'
    + '              </div>'
    + '            </div>'
    + '          </form>'
    + '          <speculate-table'
    + '            v-bind:cmd="command"'
    + '            v-if="speculating"'
    + '            v-on:reserve="reserveSpec($event)"'
    + '          ></speculate-table>'
    + '          <div class="card commandline mdl" v-if="!speculating">'
    + '            <code'
    + '              class="mdl"'
    + '              id="commandline"'
    + '              style="color: seagreen;"'
    + '            >{{ command }}</code>'
    + '          </div>'
    + '        </div>'
    + '        <div class="modal-footer m-3 mdl">'
    + '          <button'
    + '            class="modalbtn igorbtn btn btn-secondary mr-auto mdl"'
    + '            type="button"'
    + '            v-if="speculating"'
    + '            v-on:click="speculating = false"'
    + '          >Back</button>'
    + '          <button'
    + '            class="modalbtn igorbtn btn btn-secondary mr-auto mdl cancel"'
    + '            data-dismiss="modal"'
    + '            type="button"'
    + '            v-if="!speculating"'
    + '          >Cancel</button>'
    + '          <button'
    + '            :disabled="!validForm"'
    + '            class="modalbtn newresmodalgobtn igorbtn btn btn-primary mdl modalcommand speculate"'
    + '            style="background-color: #ff902d; border-color: #ff902d;"'
    + '            type="button"'
    + '            v-if="!speculating"'
    + '            v-on:click="speculating = !speculating"'
    + '          >'
    + '            <span class="mdl mdlcmdtext speculate">Speculate</span>'
    + '          </button>'
    + '          <button'
    + '            :disabled="!validForm"'
    + '            class="modalbtn newresmodalgobtn igorbtn btn btn-primary mdl modalcommand"'
    + '            style="background-color: #a975d6; border-color: #a975d6;"'
    + '            type="button"'
    + '            v-if="!speculating"'
    + '            v-on:click="submitReservation()"'
    + '          >'
    + '            <span class="mdl mdlcmdtext">Reserve</span>'
    + '          </button>'
    + '        </div>'
    + '      </div>'
    + '    </div>'
    + '  </div>'
    + '  <loading-modal'
    + '    body="This may take some time..."'
    + '    header="Submitting reservation"'
    + '    ref="loadingModal"'
    + '  ></loading-modal>'
    + '</div>';
  window.NewReservationModal = {
    template: template,
    components: {
      SpeculateTable: SpeculateTable,
      LoadingModal: LoadingModal,
    },
    data: function data() {
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
      validForm: function validForm() {
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
      command: function command() {
        let bootFrom = '-profile '.concat(this.cobblerProfile);

        if (this.isKernelInit) {
          // Check if using cobbler or KI Pairs
          if (this.kernelpair && !(this.kernelPath && this.initrdPath)) {
            // Check whether using textbox or dropdown (text overrides dropdown)
            if (this.kernelpair.includes('(user defined)')) {
              // If using drop down check whether its a user defined or default a KI Pair
              let _i = 0;

              while (_i < userDefinedImages.length) {
                if (userDefinedImages[_i].name == this.kernelpair) {
                  bootFrom = '-k '.concat(userDefinedImages[_i].kernel, ' -i ').concat(userDefinedImages[_i].initrd);
                  break;
                }

                _i++;
              }
            } else {
              bootFrom = '-k '.concat(IMAGEPATH).concat(this.kernelpair, '.kernel -i ').concat(IMAGEPATH).concat(this.kernelpair, '.initrd');
            }
          } else {
            bootFrom = '-k '.concat(this.kernelPath, ' -i ').concat(this.initrdPath);
          }
        }

        let nodes = '-n '.concat(this.numNodes);

        if (this.isNodeList) {
          nodes = '-w '.concat(this.nodeList);
        }

        let args = '';

        if (this.cmdArgs) {
          args = ' -c '.concat(this.cmdArgs);
        }

        let after = '';

        if (this.afterDate) {
          after = ' -a '.concat(this.afterDate);
        }

        return 'igor sub -r '.concat(this.name, ' ').concat(bootFrom, ' ').concat(nodes, ' -t ').concat(this.resLength).concat(args).concat(after);
      },
    },
    methods: {
      show: function show() {
        const range = this.$store.getters.selectedRange;

        if (range != '') {
          this.numNodes = this.$store.state.selectedNodes.length;
          this.nodeList = range;
          this.isNodeList = true;
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
      reserveSpec: function reserveSpec(formattedStart) {
        this.speculating = false;
        this.afterDate = formattedStart;
      },
      searchImage: function searchImage(target, container) {
        let i;
        let found = false;

        for (i = 0; i < container.length; i++) {
          if (container[i].name == target.name) {
            found = true;
            return i;
          }
        }

        return -1;
      },
      submitReservation: function submitReservation() {
        const _this = this;

        if (this.validForm) {
          if (this.kernelPath && this.initrdPath) {
            const tmp = this.kernelPath.split('/');
            const image = {
              name: tmp[tmp.length - 1].split('.')[0] + '(user defined)',
              kernel: this.kernelPath,
              initrd: this.initrdPath,
            };

            if (this.searchImage(image, userDefinedImages == -1)) {
              userDefinedImages.push(image);
              localStorage.setItem('usrImages', JSON.stringify(userDefinedImages));
            }
          }

          this.showLoading();
          this.hide();
          console.log('Running Command');
          $.get('run/', {
            run: this.command,
          }, function(data) {
            const response = JSON.parse(data);

            _this.$store.commit('updateReservations', response.Extra);

            _this.$store.commit('setAlert', 'Reservation '.concat(_this.name, ': ').concat(response.Message));

            _this.hideLoading();
          });
        }
      },
    },
    mounted: function mounted() {
      if (localStorage.kernelPath) {
        this.kernelPath = localStorage.kernelPath;
      }

      if (localStorage.initrdPath) {
        this.initrdPath = localStorage.initrdPath;
      }

      if (localStorage.getItem('usrImages')) {
        userDefinedImages = JSON.parse(localStorage.getItem('usrImages'));

        for (i = 0; i < userDefinedImages.length; i++) {
          if (this.searchImage(userDefinedImages[i], IMAGES) == -1) {
            IMAGES.push(userDefinedImages[i]);
          }
        }
      }
    },
    watch: {
      kernelPath: function kernelPath(newkernelPath) {
        localStorage.kernelPath = newkernelPath;
      },
      initrdPath: function initrdPath(newinitrdPath) {
        localStorage.initrdPath = newinitrdPath;
      },
    },
  };
})();
