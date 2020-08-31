<template>
  <div class="content">
    <b-modal :active.sync="createModal.active" :on-cancel="resetCreateModal" has-modal-card>
      <div class="modal-card" style="width: auto">
        <header class="modal-card-head">
          <p class="modal-card-title">Create a New Experiment</p>
        </header>
        <section class="modal-card-body">
          <b-field label="Experiment Name" 
            :type="createModal.nameErrType" 
            :message="createModal.nameErrMsg"  
            autofocus>
            <b-input type="text" v-model="createModal.name" v-focus></b-input>
          </b-field>
          <b-field label="Experiment Topology">
            <b-select placeholder="Select a topology" v-model="createModal.topology" expanded>
              <option v-for="( t, index ) in topologies" :key="index" :value="t">
                {{ t }}
              </option>
            </b-select>
          </b-field>
          <b-tooltip label="this list is scrollable and supports multiple selections, 
                            you can select or unselect applications before submitting 
                            the form" 
                            type="is-light is-right" 
                            multilined>
            <b-field label="Applications"></b-field>
            <b-icon icon="question-circle" style="color:#383838"></b-icon>
          </b-tooltip>
          <b-select @input="( value ) => addApp( value )" expanded>
            <option v-for="( a, index ) in applications" :key="index" :value="a">
              {{ a }}
            </option>
          </b-select>
          <b-tag v-for="( p, index ) in createModal.apps" 
                 :key="index" 
                 type="is-light" 
                 closable 
                 @close="createModal.apps.splice( index, 1 )">
            {{ p }}
          </b-tag>
          <br><br>
          <b-field label="VLAN Range">
            <b-field>
              <b-input type="number" min="1" max="4094" placeholder="minimum" v-model="createModal.vlan_min" expanded></b-input>
              <b-input type="number" min="1" max="4094" placeholder="maximum" v-model="createModal.vlan_max" expanded></b-input>
            </b-field>
          </b-field>
        </section>
        <footer class="modal-card-foot buttons is-right">
          <button class="button is-light" :disabled="!validate" @click="create">Create Experiment</button>
        </footer>
      </div>
    </b-modal>
    <template v-if="experiments.length == 0">
      <section class="hero is-light is-bold is-large">
        <div class="hero-body">
          <div class="container" style="text-align: center">
            <h1 class="title">
              There are no experiments!
            </h1>
              <b-button v-if="adminUser()" type="is-success" outlined @click="updateTopologies(); updateApplications(); createModal.active = true">Create One Now!</b-button>
          </div>
        </div>
      </section>
    </template>
    <template v-else>
      <hr>
      <b-field position="is-right">
        <b-autocomplete v-model="searchName"
                        placeholder="Find an Experiment"
                        icon="search"
                        :data="filteredData"
                        @select="option => filtered = option">
          <template slot="empty">
            No results found
          </template>
        </b-autocomplete>
        <p class='control'>
          <button class='button' style="color:#686868" @click="searchName = ''">
            <b-icon icon="window-close"></b-icon>
          </button>
        </p>
        &nbsp; &nbsp;
        <p v-if="globalUser()" class="control">
          <b-tooltip label="create a new experiment" type="is-light is-left" multilined>
            <button class="button is-light" @click="updateTopologies(); updateApplications(); createModal.active = true">
              <b-icon icon="plus"></b-icon>
            </button>
          </b-tooltip>
        </p>
      </b-field>
      <div>
        <b-table
          :data="filteredExperiments"
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
            <b-table-column field="name" label="Name" width="200" sortable>
              <template v-if="updating( props.row.status )">
                {{ props.row.name }}
              </template>
              <template v-else>
                <router-link class="navbar-item" :to="{ name: 'experiment', params: { id: props.row.name }}">
                  {{ props.row.name }}
                </router-link>
              </template>
            </b-table-column>
            <b-table-column field="status" label="Status" width="100" sortable centered>
              <template v-if="props.row.status == 'starting'">
                <section>
                  <b-progress size="is-medium" type="is-warning" show-value :value=props.row.percent format="percent"></b-progress>
                </section>
              </template>
              <template v-else-if="adminUser()">
                <span class="tag is-medium" :class="decorator( props.row.status )">
                  <div class="field">
                    <div class="field" @click="( props.row.running ) ? stop( props.row.name, props.row.status ) : start( props.row.name, props.row.status )">
                      {{ props.row.status }}
                    </div>
                  </div>
                </span>
              </template>
              <template v-else>
                <span class="tag is-medium" :class="decorator( props.row.status )">
                  {{ props.row.status }}
                </span>
              </template>
            </b-table-column>
            <b-table-column field="topology" label="Topology" width="200">
              {{ props.row.topology | lowercase }}
            </b-table-column>
            <b-table-column field="apps" label="Applications" width="200">
              {{ props.row.apps | stringify | lowercase }}
            </b-table-column>
            <b-table-column field="start_time" label="Start Time" width="250" sortable>
              {{ props.row.start_time }}
            </b-table-column>
            <b-table-column field="vm_count" label="# of VMs" width="100" centered sortable>
              {{ props.row.vm_count }}
            </b-table-column>
            <b-table-column field="vlan_range" label="VLAN Range" width="100" centered>
              {{ props.row.vlan_min }} - {{ props.row.vlan_max}}
            </b-table-column>
            <b-table-column field="vlan_count" label="Total VLANs" width="100" centered>
              {{ props.row.vlan_count }}
            </b-table-column>
            <b-table-column v-if="globalUser()" label="Delete" width="50" centered>
              <button :disabled="updating( props.row.status )" class="button is-light is-small" @click="del( props.row.name, props.row.running )">
                <b-icon icon="trash"></b-icon>
              </button>
            </b-table-column>
          </template>
        </b-table>
        <br>
        <b-field v-if="paginationNeeded" grouped position="is-right">
          <div class="control is-flex">
            <b-switch v-model="table.isPaginated" size="is-small" type="is-light">Pagenate</b-switch>
          </div>
        </b-field>
      </div>
    </template>
    <b-loading :is-full-page="true" :active.sync="isWaiting" :can-cancel="false"></b-loading>
  </div>
</template>

<script>
  export default {
    async beforeDestroy () {
      this.$options.sockets.onmessage = null;
    },

    async created () {
      this.$options.sockets.onmessage = this.handler;
      this.updateExperiments();
    },

    computed: {
      filteredExperiments: function() {
        let experiments = this.experiments;
        
        var name_re = new RegExp( this.searchName, 'i' );
        var data = [];
        
        for ( let i in experiments ) {
          let exp = experiments[i];
          if ( exp.name.match( name_re ) ) {
            exp.start_time = exp.start_time == '' ? 'N/A' : exp.start_time;
            data.push( exp );
          }
        }

        return data;
      },
    
      filteredData () {
        let names = this.experiments.map( exp => { return exp.name; } );

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
        var experiments = this.experiments;
        if ( experiments.length <= 10 ) {
          return false;
        } else {
          return true;
        }
      },
      
      validate () {
        if ( !this.createModal.name ) {
          return false;
        }

        for ( let i = 0; i < this.experiments.length; i++ ) {
          if ( this.experiments[i].name == this.createModal.name ) {
            this.createModal.nameErrType = 'is-danger';
            this.createModal.nameErrMsg  = 'experiment with this name already exists';
            return false
          }
        }

        if ( /\s/.test( this.createModal.name ) ) {
          this.createModal.nameErrType = 'is-danger';
          this.createModal.nameErrMsg  = 'experiment names cannot have a space';
          return false
        } else if ( /[A-Z]/.test( this.createModal.name ) ) {
          this.createModal.nameErrType = 'is-danger';
          this.createModal.nameErrMsg  = 'experiment names cannot have an upper case letter';
        } else if ( this.createModal.name == "create" ) {
          this.createModal.nameErrType = 'is-danger';
          this.createModal.nameErrMsg  = 'experiment names cannot be create!';
        } else {
          this.createModal.nameErrType = null;
          this.createModal.nameErrMsg  = null;
        }

        if ( !this.createModal.topology ) {
          return false;
        }

        if ( this.createModal.vlan_max < this.createModal.vlan_min ) {
          return false;
        }

        if ( this.createModal.vlan_min < 0 ) {
          return false;
        }

        if ( this.createModal.vlan_min > 4094 ) {
          return false;
        }

        if ( this.createModal.vlan_max < 0 ) {
          return false;
        }

        if ( this.createModal.vlan_max > 4094 ) {
          return false;
        }

        return true;
      }
    },
    
    methods: { 
      handler ( event ) {
        event.data.split(/\r?\n/).forEach( m => {
          let msg = JSON.parse( m );
          this.handle( msg );
        });
      },
      
      handle ( msg ) {     
        // We only care about publishes pertaining to an experiment resource.
        if ( msg.resource.type != 'experiment' ) {
          return;
        }

        let exp = this.experiments;

        switch ( msg.resource.action ) {
          case 'create': {
            msg.result.status = 'stopped';
            exp.push( msg.result );

            this.experiments = [ ...exp ];
        
            this.$buefy.toast.open({
              message: 'The ' + msg.resource.name + ' experiment has been created.',
              type: 'is-success',
              duration: 4000
            });

            break;
          }

          case 'delete': {
            for ( let i = 0; i < exp.length; i++ ) {
              if ( exp[i].name == msg.resource.name ) {
                exp.splice( i, 1 );

                break;
              }
            }
        
            this.experiments = [ ...exp ];
          
            this.$buefy.toast.open({
              message: 'The ' + msg.resource.name + ' experiment has been deleted.',
              type: 'is-success',
              duration: 4000
            });

            break;
          }

          case 'start': {
            for ( let i = 0; i < exp.length; i++ ) {
              if ( exp[i].name == msg.resource.name ) {
                exp[i] = msg.result;
                exp[i].status = 'started';

                break;
              }
            }
          
            this.experiments = [ ...exp ];
          
            this.$buefy.toast.open({
              message: 'The ' + msg.resource.name + ' experiment has been started.',
              type: 'is-success',
              duration: 4000
            });

            break;
          }

          case 'stop': {
            for ( let i = 0; i < exp.length; i++ ) {
              if ( exp[i].name == msg.resource.name ) {
                exp[i] = msg.result;
                exp[i].status = 'stopped';

                break;
              }
            }
          
            this.experiments = [ ...exp ];
          
            this.$buefy.toast.open({
              message: 'The ' + msg.resource.name + ' experiment has been stopped.',
              type: 'is-success',
              duration: 4000
            });

            break;
          }

          case 'starting': // fallthru to `stopping`
          case 'stopping': {
            for ( let i = 0; i < exp.length; i++ ) {
              if ( exp[i].name == msg.resource.name ) {
                exp[i].status = msg.resource.action;
                exp[i].percent = 0;

                break;
              }
            }
            
            this.experiments = [ ...exp ];
            
            this.$buefy.toast.open({
              message: 'The ' + msg.resource.name + ' experiment is being updated.',
              type: 'is-warning'
            });

            break;
          }

          case 'progress': {
            let percent = ( msg.result.percent * 100 ).toFixed( 0 );
            
            for ( let i = 0; i < exp.length; i++ ) {
              if ( exp[i].name == msg.resource.name ) {
                exp[i].percent = parseInt( percent );
                break;
              }
            }
            
            this.experiments = [ ...exp ];

            break;
          }
        }
      },
       
      updateExperiments () {
        this.$http.get( 'experiments' ).then(
          response => {
            response.json().then( state => {
              this.experiments = state.experiments;
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
      
      updateTopologies () {
        this.$http.get( 'topologies' ).then(
          response => {
            response.json().then( state => {
              this.topologies = state.topologies;
              this.isWaiting = false;
            });
          }, response => {
            this.isWaiting = false;
            this.$buefy.toast.open({
              message: 'Getting the topologies failed.',
              type: 'is-danger',
              duration: 4000
            });
          }
        );
      },
      
      updateApplications () {
        this.$http.get( 'applications' ).then(
          response => {
            response.json().then( state => {
              this.applications = state.applications;
              this.isWaiting = false;
            });
          }, response => {
            this.isWaiting = false;
            this.$buefy.toast.open({
              message: 'Getting the applications failed.',
              type: 'is-danger',
              duration: 4000
            });
          }
        );
      },

      globalUser () {
        return [ 'Global Admin' ].includes( this.$store.getters.role );
      },
      
      adminUser () {
        return [ 'Global Admin', 'Experiment Admin' ].includes( this.$store.getters.role );
      },
      
      experimentUser () {
        return [ 'Global Admin', 'Experiment Admin', 'Experiment User' ].includes( this.$store.getters.role );
      },

      update: function ( value ) {
        this.isMenuActive = true;
      },

      updating: function( status ) {
        return status === "starting" || status === "stopping";
      },
      
      decorator ( status ) {
        switch ( status ) {
          case 'started':
            return 'is-success';
          case 'starting':
          case 'stopping':
            return 'is-warning';
          case 'stopped':
            return 'is-danger';
        }
      },
      
      start ( name, status ) {
        if ( status == 'starting' || status == 'stopping' ) {
          this.$buefy.toast.open({
            message: 'The ' + name + ' experiment is currently ' + status + '. You cannot make any changes at this time.',
            type: 'is-warning'
          });
          
          return;
        }
        
        this.$buefy.dialog.confirm({
          title: 'Start the Experiment',
          message: 'This will start the ' + name + ' experiment.',
          cancelText: 'Cancel',
          confirmText: 'Start',
          type: 'is-success',
          hasIcon: true,
          onConfirm: async () => {
            try {
              await this.$http.post('experiments/' + name + '/start');
              console.log('experiment started');
            } catch (err) {
              this.$buefy.toast.open({
                message: 'Starting the ' + name + ' experiment failed with ' + err.status + ' status.',
                type: 'is-danger',
                duration: 4000
              });

              this.isWaiting = false;
            }
          }
        });
      },
      
      stop ( name, status ) {
        if ( status == 'starting' || status == 'stopping' ) {
          this.$buefy.toast.open({
            message: 'The ' + name + ' experiment is currently ' + status + '. You cannot make any changes at this time.',
            type: 'is-warning'
          });
          
          return;
        }

        this.$buefy.dialog.confirm({
          title: 'Stop the Experiment',
          message: 'This will stop the ' + name + ' experiment.',
          cancelText: 'Cancel',
          confirmText: 'Stop',
          type: 'is-danger',
          hasIcon: true,
          onConfirm: () => {
            this.$http.post(
              'experiments/' + name + '/stop'
            ).then(
              response => { 
                console.log('experiment stopped');
              }, response => {
                this.$buefy.toast.open({
                  message: 'Stopping the ' + name + ' experiment failed with ' + response.status + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
                
                this.isWaiting = false;
              }
            )
          }
        })
      },
      
      del ( name, running ) {
        if ( running ) {
          this.$buefy.toast.open({
            message: 'The ' + name + ' experiment is running; you must stop it before deleting it.',
            type: 'is-warning',
            duration: 4000
          });
        } else {
          this.$buefy.dialog.confirm({
            title: 'Delete the Experiment',
            message: 'This will DELETE the ' + name + ' experiment. Are you sure you want to do this?',
            cancelText: 'Cancel',
            confirmText: 'Delete',
            type: 'is-danger',
            hasIcon: true,
            onConfirm: () => {
              this.isWaiting = true;

              this.$http.delete(
                'experiments/' + name
              ).then(
                response => {
                  if ( response.status == 204 ) {
                    let exp = this.experiments;
                
                    for ( let i = 0; i < exp.length; i++ ) {
                      if ( exp[i].name == name ) {
                        exp.splice( i, 1 );
                        break;
                      }
                    }
                  
                    this.experiments = [ ...exp ];
                  }
                
                  this.isWaiting = false;
                }, response => {
                  this.$buefy.toast.open({
                    message: 'Deleting the ' + name + ' experiment failed with ' + response.status + ' status.',
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
      
      create () {      
        const experimentData = {
          name: this.createModal.name,
          topology: this.createModal.topology,
          apps: this.createModal.apps,
          vlan_min: +this.createModal.vlan_min,
          vlan_max: +this.createModal.vlan_max
        }
        
        var appsUnique = Array.from(new Set( experimentData.apps ));
        experimentData.apps = appsUnique;
        
        if ( !this.createModal.name ) {
          this.$buefy.toast.open({
            message: 'You must include a name for the experiment.',
            type: 'is-warning',
            duration: 4000
          });
          
          return {}
        }
        
        if ( !this.createModal.topology ) {
          this.$buefy.toast.open({
            message: 'You must select an experiment topology.',
            type: 'is-warning',
            duration: 4000
          });
          
          return {}
        }

        this.isWaiting= true;
        
        this.$http.post(
          'experiments', experimentData, { timeout: 0 } 
        ).then(
          response => {            
            this.isWaiting = false;
          }, response => {
            this.$buefy.toast.open({
              message: 'Creating the ' + experimentData.name + ' experiment failed with ' + response.status + ' status.',
              type: 'is-danger',
              duration: 4000
            });
            
            this.isWaiting = false;
          }
        );

        this.createModal.active = false;
        this.resetCreateModal();
      },

      addApp ( app ) {
        this.createModal.apps.push( app )
        this.createModal.apps = _.uniq( this.createModal.apps )
      },
      
      resetCreateModal () {
        this.createModal = {
          active: false,
          name: null,
          nameErrType: null,
          nameErrMsg: null,
          topology: null,
          apps: [],
          vlan_min: null,
          vlan_max: null
        }
      }
    },
    
    directives: {
      focus: {
        inserted ( el ) {
          if ( el.tagName == 'INPUT' ) {
            el.focus()
          } else {
            el.querySelector( 'input' ).focus()
          }
        }
      }
    },

    data () {
      return {
        table: {
          isPaginated: true,
          perPage: 10,
          currentPage: 1,
          isPaginationSimple: true,
          paginationSize: 'is-small',
          defaultSortDirection: 'asc'
        },
        createModal: {
          active: false,
          name: null,
          nameErrType: null,
          nameErrMsg: null,
          topology: null,
          apps: [],
          vlan_min: null,
          vlan_max: null
        },
        experiments: [],
        topologies: [],
        applications: [],
        searchName: '',
        filtered: null,
        isMenuActive: false,
        action: null,
        rowName: null,
        isWaiting: true
      }
    }
  }
</script>

<style scoped>
  div.autocomplete >>> a.dropdown-item {
    color: #383838 !important;
  }
</style>
