<template>
  <div class="content">
    <b-modal :active.sync="expModal.active" has-modal-card>
      <div class="modal-card" style="width:25em">
        <header class="modal-card-head">
          <p class="modal-card-title">VM {{ expModal.vm.name ? expModal.vm.name : "unknown" }}</p>
        </header>
        <section class="modal-card-body">
          <p>Host: {{ expModal.vm.host }}</p>
          <p>IPv4: {{ expModal.vm.ipv4 | stringify }}</p>
          <p>CPU(s): {{ expModal.vm.cpus }}</p>
          <p>Memory: {{ expModal.vm.ram | ram }}</p>
          <p>Disk: {{ expModal.vm.disk }}</p>
          <p>Uptime: {{ expModal.vm.uptime | uptime }}</p>
          <p>Network(s): {{ expModal.vm.networks | stringify | lowercase }}</p>
          <p>Taps: {{ expModal.vm.taps | stringify | lowercase }}</p>
        </section>
        <footer class="modal-card-foot">
        </footer>
      </div>
    </b-modal>
    <hr>
    <div class="level is-vcentered">
      <div class="level-item">
        <span style="font-weight: bold; font-size: x-large;">Experiment: {{ this.$route.params.id }}</span>&nbsp;
      </div>
      <div class="level-item" v-if="experiment.scenario">
        <span style="font-weight: bold;">Scenario: {{ experiment.scenario }}</span>&nbsp;
      </div>
      <div class="level-item" v-if="experiment.scenario">
        <span style="font-weight: bold;">Apps:</span>&nbsp;
        <b-taglist>
          <b-tag v-for="( a, index ) in experiment.apps" :key="index" type="is-light">
            {{ a }}  
          </b-tag>
        </b-taglist>
      </div>
    </div>
    <b-field v-if="experimentUser() || experimentViewer()" position="is-right">
      <b-autocomplete
        v-model="searchName"
        placeholder="Find a VM"
        icon="search"
        :data="filteredData"
        @select="option => filtered = option">
          <template slot="empty">No results found</template>
      </b-autocomplete>
      <p class='control'>
        <button class='button' style="color:#686868" @click="searchName = ''">
          <b-icon icon="window-close"></b-icon>
        </button>
      </p>
      &nbsp; &nbsp;
      <p class="control buttons">
        <b-button v-if="adminUser()" class="button is-success" slot="trigger" icon-right="play" @click="start"></b-button>
      </p>
      &nbsp; &nbsp;
      <p class="control">
        <b-tooltip label="menu for scheduling hosts to the experiment" type="is-light" multilined>
          <b-dropdown v-model="algorithm" class="is-right" aria-role="list">
            <button class="button is-light" slot="trigger">
              <b-icon icon="bars"></b-icon>
            </button>
            <b-dropdown-item v-for="( s, index ) in schedules" :key="index" :value="s" @click="updateSchedule">
              <font color="#202020">{{ s }}</font>
            </b-dropdown-item>
          </b-dropdown>
        </b-tooltip>
      </p>  
    </b-field>
    <div style="margin-top: -5em;">
      <b-tabs @change="updateFiles">
        <b-tab-item label="Table">
          <b-table
            :key="table.key"
            :data="vms"
            :paginated="table.isPaginated && paginationNeeded"
            :per-page="table.perPage"
            :current-page.sync="table.currentPage"
            :pagination-simple="table.isPaginationSimple"
            :pagination-size="table.paginationSize"
            :default-sort-direction="table.defaultSortDirection"
            default-sort="name">
              <template slot="empty">
                <section class="section">
                  <div class="content has-text-white has-text-centered">
                    Your search turned up empty!
                  </div>
                </section>
              </template>
              <template slot-scope="props">
                <b-table-column field="name" label="VM" sortable>
                  <template v-if="adminUser()">
                    <b-tooltip label="get info on the vm" type="is-dark">
                      <div class="field">
                        <div @click="expModal.active = true; expModal.vm = props.row">
                          {{ props.row.name }}
                        </div>
                      </div>
                    </b-tooltip>
                  </template>
                  <template v-else>
                    {{ props.row.name }}
                  </template>
                </b-table-column>
                <b-table-column field="host" label="Host" width="200" sortable>
                  <template v-if="adminUser()">
                    <b-tooltip label="assign the vm to a specific host" type="is-dark">
                      <b-field>
                        <b-select :value="props.row.host" expanded @input="( value ) => assignHost( props.row.name, value )">
                          <option
                            v-for="( h, index ) in hosts"
                            :key="index"
                            :value="h">
                            {{ h }}
                          </option>
                        </b-select>
                        <p class='control'>
                          <button class='button' 
                                  @click="unassignHost( props.row.name )">
                            <b-icon icon="window-close"></b-icon>
                          </button>
                        </p>
                      </b-field>
                    </b-tooltip>
                  </template>
                  <template v-else>
                    {{ props.row.host }}
                  </template>
                </b-table-column>
                <b-table-column field="ipv4" label="IPv4">
                  <div v-for="ip in props.row.ipv4">
                    {{ ip }}
                  </div>
                </b-table-column>
                <b-table-column field="cpus" label="CPUs" sortable centered>
                  <template v-if="adminUser()">
                    <b-tooltip label="menu for assigning vm(s) cpus" type="is-dark">
                      <b-select :value="props.row.cpus" expanded @input="( value ) => assignCpu( props.row.name, value )">
                        <option value="1">1</option>
                        <option value="2">2</option>
                        <option value="3">3</option>
                        <option value="4">4</option>
                      </b-select>
                    </b-tooltip>
                  </template>
                  <template v-else>
                    {{ props.row.cpus }}
                  </template>
                </b-table-column>
                <b-table-column field="ram" label="Memory" sortable centered>
                  <template v-if="adminUser()">
                    <b-tooltip label="menu for assigning vm(s) memory" type="is-dark">
                      <b-select :value="props.row.ram" expanded @input="( value ) => assignRam( props.row.name, value )">
                        <option value="512">512 MB</option>
                        <option value="1024">1 GB</option>
                        <option value="2048">2 GB</option>
                        <option value="3072">3 GB</option>
                        <option value="4096">4 GB</option>
                        <option value="8192">8 GB</option>
                        <option value="12288">12 GB</option>
                        <option value="16384">16 GB</option>
                      </b-select>
                    </b-tooltip>
                  </template>
                  <template v-else>
                    {{ props.row.ram }}
                  </template>
                </b-table-column>
                <b-table-column field="disk" label="Disk">
                  <template v-if="adminUser()">
                    <b-tooltip label="menu for assigning vm(s) disk" type="is-dark">
                      <b-select :value="props.row.disk" @input="( value ) => assignDisk( props.row.name, value )">
                        <option
                          v-for="( d, index ) in disks"
                          :key="index"
                          :value="d">
                            {{ d }}
                        </option>
                      </b-select>
                    </b-tooltip>
                  </template>
                  <template v-else>
                    {{ props.row.disk }}
                  </template>
                </b-table-column>
                <b-table-column v-if="experimentUser()" label="Boot" centered>
                  <b-tooltip label="control whether or not VM boots" type="is-dark">
                    <div @click="updateDnb(props.row.name, !props.row.dnb)">
                      <font-awesome-icon :class="bootDecorator(props.row.dnb)" icon="bolt" />
                    </div>
                  </b-tooltip>
                </b-table-column>
              </template>
          </b-table>
          <br>
          <b-field v-if="paginationNeeded" grouped position="is-right">
            <div class="control is-flex">
              <b-switch v-model="table.isPaginated" size="is-small" type="is-light">Pagenate</b-switch>
            </div>
          </b-field>
        </b-tab-item>
        <b-tab-item label="Files">
          <template v-if="files && !files.length">
            <section class="hero is-light is-bold is-large">
              <div class="hero-body">
                <div class="container" style="text-align: center">
                  <h1 class="title">
                    There are no files available.
                  </h1>
                </div>
              </div>
            </section>
          </template>
          <template v-else>
            <ul class="fa-ul" style="list-style:none">
              <li v-for="( f, index ) in files" :key="index">
                <font-awesome-icon class="fa-li" icon="file-download" />
                <a :href="'/api/v1/experiments/'
                          + experiment.name 
                          + '/files/' 
                          + f 
                          + '?token=' 
                          + $store.state.token" target="_blank">
                  {{ f }}
                </a>
              </li>
            </ul>
          </template>
        </b-tab-item>
      </b-tabs>
    </div>
    <b-loading :is-full-page="true" :active.sync="isWaiting" :can-cancel="false"></b-loading>
  </div>
</template>

<script>
  export default {
    beforeDestroy () {
      this.$options.sockets.onmessage = null;
    },

    async created () {
      this.$options.sockets.onmessage = this.handler;
      this.updateExperiment();
      
      if ( this.adminUser() ) {
        this.updateHosts();
        this.updateDisks();      
      }
    },

    computed: {
      vms: function() {
        let vms = this.experiment.vms;
        
        var name_re = new RegExp( this.searchName, 'i' );
        var data = [];
        
        for ( let i in vms ) {
          let vm = vms[i];
          if ( vm.name.match( name_re ) ) {
            data.push( vm );
          }
        }
        
        return data;
      },

      filteredData () {
        let names = this.vms.map( vm => { return vm.name; } );
        
        return names.filter(
          option => {
            return option
              .toString()
              .toLowerCase()
              .indexOf( this.searchName.toLowerCase() ) >= 0
          }
        )
      },

      paginationNeeded () {
        if ( this.vms.length <= 10 ) {
          return false;
        }

        return true;
      }
    },

    methods: {
      adminUser () {
        return [ 'Global Admin', 'Experiment Admin' ].includes( this.$store.getters.role );
      },

      experimentUser () {
        return [ 'Global Admin', 'Experiment Admin', 'Experiment User' ].includes( this.$store.getters.role );
      },

      experimentViewer () {
        return [ 'Experiment Viewer' ].includes( this.$store.getters.role );
      },

      bootDecorator ( dnb ) {
        if ( dnb ) {
          return '';
        } else {
          return 'boot';
        }
      },

      handler ( event ) {
        event.data.split(/\r?\n/).forEach( m => {
          let msg = JSON.parse( m );
          this.handle( msg );
        });
      },
    
      handle ( msg ) {
        switch ( msg.resource.type ) {
          case 'experiment': {
            // We only care about experiment publishes pertaining to the
            // schedule action when in a stopped experiment.
            if ( msg.resource.action != 'schedule' ) {
              return;
            }

            let vms = this.experiment.vms;

            for ( let i = 0; i < msg.result.schedule.length; i++ ) {
              for ( let j = 0; i < vms.length; j++ ) {
                if ( vms[j].name == msg.result.schedule[i].vm ) {
                  vms[j].host = msg.result.schedule[i].host;
                  break;
                }
              }
            }

            this.experiment.vms = [ ...vms ];
          
            this.$buefy.toast.open({
              message: 'The VMs for this experiment have been scheduled.',
              type: 'is-success'
            });

            break;
          }

          case 'experiment/vm': {
            // We only care about experiment VM publishes pertaining to
            // the update action when in a stopped experiment.
            if ( msg.resource.action != 'update' ) {
              return;
            }

            let vms = this.experiment.vms;

            for ( let i = 0; i < vms.length; i++ ) {
              if ( vms[i].name == msg.result.name ) {
                vms[i] = msg.result;

                break;
              }
            }
        
            this.experiment.vms = [ ...vms ];
          
            this.$buefy.toast.open({
              message: 'The VM ' + msg.result.name + ' has been successfully updated.',
              type: 'is-success'
            });            

            break;
          }
        }
      },
      
      updateExperiment () {
        this.$http.get( 'experiments/' + this.$route.params.id ).then(
          response => {
            response.json().then( state => {
              this.experiment = state;
              this.isWaiting = false;
            });
          }, response => {
            this.isWaiting = false;
            this.$buefy.toast.open({
              message: 'Getting the experiments failed.',
              type: 'is-danger',
              duration: 4000
            });
          }
        );
      },
      
      updateHosts () {
        this.$http.get( 'hosts' ).then(
          response => {
            response.json().then(
              state => {
                if ( state.hosts.length == 0 ) {
                  this.isWaiting = true;
                } else {
                  for ( let i = 0; i < state.hosts.length; i++ ) {
                    if ( state.hosts[i].schedulable ) {
                      this.hosts.push( state.hosts[i].name );
                    }
                  }
                  
                  this.isWaiting = false;
                }
              }
            );
          }, response => {
            this.isWaiting = false;
            this.$buefy.toast.open({
              message: 'Getting the hosts failed.',
              type: 'is-danger',
              duration: 4000
            });
          }
        );
      },
      
      updateDisks () {
        this.$http.get( 'disks' ).then(
          response => {
            response.json().then(
              state => {
                if ( state.disks.length == 0 ) {
                  this.isWaiting = true;
                } else {
                  for ( let i = 0; i < state.disks.length; i++ ) {
                    this.disks.push( state.disks[i] );
                  }
                  
                  this.isWaiting = false;
                }
              }
            );
          }, response => {
            this.isWaiting = false;
            this.$buefy.toast.open({
              message: 'Getting the disks failed.',
              type: 'is-danger',
              duration: 4000
            });
          }
        );
      },
      
      updateFiles () {
        this.$http.get( 'experiments/' + this.$route.params.id + '/files' ).then(
          response => {
            response.json().then(
              state => {
                for ( let i = 0; i < state.files.length; i++ ){
                  this.files.push( state.files[i] );
                }
                
                this.isWaiting = false;
              }
            );
          }, response => {
            this.isWaiting = false;
            this.$buefy.toast.open({
              message: 'Getting the files failed.',
              type: 'is-danger',
              duration: 4000
            });
          }
        );
      },

      start () {
        this.$buefy.dialog.confirm({
          title: 'Start the Experiment',
          message: 'This will start the ' + this.$route.params.id + ' experiment.',
          cancelText: 'Cancel',
          confirmText: 'Start',
          type: 'is-success',
          hasIcon: true,
          onConfirm: () => {
            this.isWaiting = true;

            this.$http.post(
              'experiments/' + this.$route.params.id + '/start'
            ).then(
              response => { 
                console.log('the ' + this.$route.params.id + ' experiment was started.');
                        
                this.$router.replace('/experiments/');
              }, response => {
                this.$buefy.toast.open({
                  message: 'Starting experiment ' + this.$route.params.id + ' failed with ' + response.status + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
                
                this.isWaiting = false;
              }
            );
          }
        })
      },

      assignHost ( name, host ) {        
        this.$buefy.dialog.confirm({
          title: 'Assign a Host',
          message: 'This will assign the ' + name + ' VM to the ' + host + ' host.',
          cancelText: 'Cancel',
          confirmText: 'Assign Host',
          type: 'is-success',
          hasIcon: true,
          onConfirm: () => {
            this.isWaiting = true;
            
            let update = { "host": host };
            
            this.$http.patch(
              'experiments/' + this.$route.params.id + '/vms/' + name, update
            ).then(
              response => {
                let vms = this.experiment.vms;
                
                for ( let i = 0; i < vms.length; i++ ) {
                  if ( vms[i].name == response.body.name ) {
                    vms[i] = response.body;
                    break;
                  }
                }
              
                this.experiment.vms = [ ...vms ];
              
                this.isWaiting = false;
              }, response => {
                this.$buefy.toast.open({
                  message: 'Assigning the ' 
                           + name 
                           + ' VM to the ' 
                           + host 
                           + ' host failed with ' 
                           + response.status 
                           + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
                
                this.isWaiting = false;
              }
            )
          },
          onCancel: () => {
            // force table to be rerendered so selected value resets
            this.tableKey += 1;
          }
        })
      },

      unassignHost ( name ) {
        this.$buefy.dialog.confirm({
          title: 'Unassign a Host',
          message: 'This will cancel the host assignment for ' + name + ' VM.',
          cancelText: 'Cancel',
          confirmText: 'Unassign Host',
          type: 'is-success',
          hasIcon: true,
          onConfirm: () => {
            this.isWaiting = true;
            
            let update = { "host": ''};

            this.$http.patch(
              'experiments/' + this.$route.params.id + '/vms/' + name, update
            ).then(
              response => {
                let vms = this.experiment.vms;
                
                for ( let i = 0; i < vms.length; i++ ) {
                  if ( vms[i].name == response.body.name ) {
                    vms[i] = response.body;
                    break;
                  }
                }
              
                this.experiment.vms = [ ...vms ];
              
                this.isWaiting = false;              
              }, response => {
                this.$buefy.toast.open({
                  message: 'Canceling the ' 
                           + host 
                           + ' assignment for the ' 
                           + name 
                           + ' VM failed with ' 
                           + response.status 
                           + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
                
                this.isWaiting = false;
              }
            )
          }
        })
      },

      assignCpu ( name, cpus ) {
        this.$buefy.dialog.confirm({
          title: 'Assign CPUs',
          message: 'This will assign ' + cpus + ' cpu(s) to the ' + name + ' VM.',
          cancelText: 'Cancel',
          confirmText: 'Assign CPUs',
          type: 'is-success',
          hasIcon: true,
          onConfirm: () => {
            this.isWaiting = true;
            
            let update = { "cpus": cpus };

            this.$http.patch(
              'experiments/' + this.$route.params.id + '/vms/' + name, update
            ).then(
              response => {
                let vms = this.experiment.vms;
                
                for ( let i = 0; i < vms.length; i++ ) {
                  if ( vms[i].name == response.body.name ) {
                    vms[i] = response.body;
                    break;
                  }
                }
              
                this.experiment.vms = [ ...vms ];
              
                this.isWaiting = false;              
              }, response => {
                this.$buefy.toast.open({
                  message: 'Assigning ' 
                           + cpus 
                           + ' cpu(s) to the ' 
                           + name 
                           + ' VM failed with ' 
                           + response.status 
                           + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
                
                this.isWaiting = false;
              }
            )
          },
          onCancel: () => {
            // force table to be rerendered so selected value resets
            this.tableKey += 1;
          }
        })
      },

      assignRam ( name, ram ) {
        this.$buefy.dialog.confirm({
          title: 'Assign Memory',
          message: 'This will assign ' + ram + ' of memory to the ' + name + ' VM.',
          cancelText: 'Cancel',
          confirmText: 'Assign Memory',
          type: 'is-success',
          hasIcon: true,
          onConfirm: () => {
            this.isWaiting = true;
            
            let update = { "ram": ram };

            this.$http.patch(
              'experiments/' + this.$route.params.id + '/vms/' + name, update
            ).then(
              response => {
                let vms = this.experiment.vms;
                
                for ( let i = 0; i < vms.length; i++ ) {
                  if ( vms[i].name == response.body.name ) {
                    vms[i] = response.body;
                    break;
                  }
                }
              
                this.experiment.vms = [ ...vms ];
              
                this.isWaiting = false;              
              }, response => {
                this.$buefy.toast.open({
                  message: 'Assigning ' 
                           + ram 
                           + ' of memory to the ' 
                           + name 
                           + ' VM failed with ' 
                           + response.status 
                           + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
                
                this.isWaiting = false;
              }
            )
          },
          onCancel: () => {
            // force table to be rerendered so selected value resets
            this.tableKey += 1;
          }
        })
      },

      assignDisk ( name, disk ) {
        this.$buefy.dialog.confirm({
          title: 'Assign a Disk Image',
          message: 'This will assign the ' + disk + ' disk image to the ' + name + ' VM.',
          cancelText: 'Cancel',
          confirmText: 'Assign Disk',
          type: 'is-success',
          hasIcon: true,
          onConfirm: () => {
            this.isWaiting = true;
            
            let update = { "disk": disk };

            this.$http.patch(
              'experiments/' + this.$route.params.id + '/vms/' + name, update
            ).then(
              response => {
                let vms = this.experiment.vms;
                
                for ( let i = 0; i < vms.length; i++ ) {
                  if ( vms[i].name == response.body.name ) {
                    vms[i] = response.body;
                    break;
                  }
                }
              
                this.experiment.vms = [ ...vms ];
              
                this.isWaiting = false;              
              }, response => {
                this.$buefy.toast.open({
                  message: 'Assigning the ' 
                           + disk 
                           + ' to the ' 
                           + name 
                           + ' VM failed with ' 
                           + response.status 
                           + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
                
                this.isWaiting = false;
              }
            )
          },
          onCancel: () => {
            // force table to be rerendered so selected value resets
            this.tableKey += 1;
          }
        })
      },

      updateDnb ( name, dnb ) {
        if ( dnb ) {
          this.$buefy.dialog.confirm({
            title: 'Set Do NOT Boot',
            message: 'This will set the ' + name + ' VM to NOT boot when the experiment starts.',
            cancelText: 'Cancel',
            confirmText: 'Do NOT Boot',
            type: 'is-warning',
            hasIcon: true,
            onConfirm: () => {
              this.isWaiting = true;
              
              let update = { "dnb": dnb };

              this.$http.patch(
                'experiments/' + this.$route.params.id + '/vms/' + name, update
              ).then(
                response => {
                  let vms = this.experiment.vms;
                
                  for ( let i = 0; i < vms.length; i++ ) {
                    if ( vms[i].name == response.body.name ) {
                      vms[i] = response.body;
                      break;
                    }
                  }
              
                  this.experiment.vms = [ ...vms ];
              
                  this.isWaiting = false;              
                }, response => {
                  this.$buefy.toast.open({
                    message: 'Setting the ' 
                             + name 
                             + ' VM to NOT boot when experiment starts failed with ' 
                             + response.status 
                             + ' status.',
                    type: 'is-danger',
                    duration: 4000
                  });
                  
                  this.isWaiting = false;
                }
              )
            }
          })
        } else {
          this.$buefy.dialog.confirm({
            title: 'Set Boot',
            message: 'This will set the ' + name + ' VM to boot when the experiment starts.',
            cancelText: 'Cancel',
            confirmText: 'Boot',
            type: 'is-success',
            hasIcon: true,
            onConfirm: () => {
              this.isWaiting = true;
              
              let update = { "dnb": dnb };

              this.$http.patch(
                'experiments/' + this.$route.params.id + '/vms/' + name, update
              ).then(
                response => {
                  let vms = this.experiment.vms;
                
                  for ( let i = 0; i < vms.length; i++ ) {
                    if ( vms[i].name == response.body.name ) {
                      vms[i] = response.body;
                      break;
                    }
                  }
              
                  this.experiment.vms = [ ...vms ];
              
                  this.isWaiting = false;              
                }, response => {                  
                  this.$buefy.toast.open({
                    message: 'Setting the ' 
                             + name 
                             + ' VM to boot when experiment starts failed with ' 
                             + response.status 
                             + ' status.',
                    type: 'is-danger',
                    duration: 4000
                  });
                  
                  this.isWaiting = false;
                }
              )
            }
          })
        }
      },

      updateSchedule () {
        this.$buefy.dialog.confirm({
          title: 'Assign a Host Schedule',
          message: 'This will schedule host(s) with the ' 
                   + this.algorithm 
                   + ' algorithm for the ' 
                   + this.$route.params.id 
                   + ' experiment.',
          cancelText: 'Cancel',
          confirmText: 'Assign Schedule',
          type: 'is-success',
          hasIcon: true,
          onConfirm: () => {
            this.isWaiting = true;

            this.$http.post(
              'experiments/' + this.$route.params.id + '/schedule', { "algorithm": this.algorithm }
            ).then(
              response => {
                let vms = this.experiment.vms;
                
                for ( let i = 0; i < vms.length; i++ ) {
                  if ( vms[i].name == response.body.name ) {
                    vms[i] = response.body;
                    break;
                  }
                }
              
                this.experiment.vms = [ ...vms ];
              
                this.isWaiting = false;              
              }, response => {
                this.$buefy.toast.open({
                  message: 'Scheduling the host(s) with the ' 
                           + this.algorithm 
                           + ' for the ' 
                           + this.$route.params.id 
                           + ' experiment failed with ' 
                           + response.status 
                           + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
                
                this.isWaiting = false;
              }
            )
          }
        })
      }
    },

    data () {
      return {
        table: {
          key: 0,
          isPaginated: true,
          perPage: 10,
          currentPage: 1,
          isPaginationSimple: true,
          paginationSize: 'is-small',
          defaultSortDirection: 'asc'
        },
        expModal: {
          active: false,
          vm: []
        },
        schedules: [
          'isolate_experiment',
          'round_robin',
        ],
        experiment: [],
        files: [],
        hosts: [],
        disks: [],
        searchName: '',
        filtered: null,
        algorithm: null,
        dnb: false,
        isWaiting: true
      }
    }
  }
</script>

<style scoped>
  b-dropdown {
    color: #383838;
  }
  
  svg.fa-bolt.boot {
    color: #c46200;
  }

  div.autocomplete >>> a.dropdown-item {
    color: #383838 !important;
  }
</style>
