<template>
  <div class="content">
    <b-modal :active.sync="isCreateActive" :on-cancel="resetLocalUser" has-modal-card>
      <div class="modal-card" style="width:25em">
        <header class="modal-card-head">
          <p class="modal-card-title">Create a New User</p>
        </header>
        <section class="modal-card-body">
          <b-field label="User Name" 
            :type="{ 'is-danger' : userExists }" 
            :message="{ 'User already exists' : userExists }">
            <b-input type="text" v-model="user.username" minlength="4" maxlength="32" autofocus></b-input>
          </b-field>
          <b-field label="First Name">
            <b-input type="text" v-model="user.first_name"></b-input>
          </b-field>
          <b-field label="Last Name">
            <b-input type="text" v-model="user.last_name"></b-input>
          </b-field>
          <b-field label="Password">
            <b-input type="password" minlength="8" maxlength="32" v-model="user.password"></b-input>
          </b-field>
          <b-field label="Confirm Password">
            <b-input type="password" minlength="8" maxlength="32" v-model="user.confirmPassword"></b-input>
          </b-field>
          <b-field label="Role">
            <b-select v-model="user.role_name" expanded>
              <option v-for="r in roles">
                {{ r }}
              </option>
            </b-select>
          </b-field>
          <b-tooltip label="resource names can include wildcards for broad  
                  assignment (ex. *inv*); they can also include `!` to not   
                  allow access to a resource (ex. !*inv*)" 
                  type="is-light is-right" 
                  multilined>            
          <b-field label="Resource Name(s)"></b-field>
          <b-icon icon="question-circle" style="color:#383838"></b-icon>
          </b-tooltip>
            <b-input type="text" v-model="user.resource_names"></b-input>
        </section>
        <footer class="modal-card-foot buttons is-right">
          <button class="button is-light" @click="createUser">Create User</button>
        </footer>
      </div>
    </b-modal>
    <b-modal :active.sync="isEditActive" :on-cancel="resetLocalUser" has-modal-card>
      <div class="modal-card" style="width:25em">
        <header class="modal-card-head">
          <p class="modal-card-title">User {{ user.username }}</p>
        </header>
        <section class="modal-card-body">
          <b-field label="First Name">
            <b-input type="text" v-model="user.first_name" autofocus></b-input>
          </b-field>
          <b-field label="Last Name">
            <b-input type="text" v-model="user.last_name"></b-input>
          </b-field>
          <b-field label="Role">
            <b-select v-model="user.role_name" expanded>
              <option v-for="r in roles">
                {{ r }}
              </option>
            </b-select>
          </b-field>
          <b-field label="Resource Name(s)">
            <b-input type="text" v-model="user.resource_names"></b-input>
          </b-field>
        </section>
        <footer class="modal-card-foot buttons is-right">
          <button class="button is-light" @click="updateUser">Update User</button>
        </footer>
      </div>
    </b-modal>
    <template>
      <hr>
      <b-field grouped position="is-right">
        <p v-if="adminUser()" class="control">
          <b-tooltip label="create a new user" type="is-light is-left">
            <button class="button is-light" @click="isCreateActive = true">
              <b-icon icon="plus"></b-icon>
            </button>
          </b-tooltip>
        </p>
      </b-field>
      <div>
        <b-table
          :data="users"
          :paginated="table.isPaginated && paginationNeeded"
          :per-page="table.perPage"
          :current-page.sync="table.currentPage"
          :pagination-simple="table.isPaginationSimple"
          :pagination-size="table.paginationSize"
          :default-sort-direction="table.defaultSortDirection"
          default-sort="username">
          <template slot-scope="props">
            <b-table-column field="username" label="User" sortable>
              <template v-if="adminUser()">
                <b-tooltip label="change user settings" type="is-dark">
                  <div class="field">
                    <div @click="editUser( props.row.username )">
                      {{ props.row.username }}
                    </div>
                  </div>
                </b-tooltip>
              </template>
              <template v-else>
                {{ props.row.username }}
              </template>
            </b-table-column>
            <b-table-column field="first_name" label="First Name">
              {{ props.row.first_name }}
            </b-table-column>
            <b-table-column field="last_name" label="Last Name" sortable>
              {{ props.row.last_name }}
            </b-table-column>
            <b-table-column field="role" label="Role" sortable>
              {{ props.row.role_name ? props.row.role_name : "Not yet assigned" }}
            </b-table-column>
            <b-table-column v-if="adminUser()" label="Delete" width="50" centered>
              <button class="button is-light is-small" @click="deleteUser( props.row.username )">
                <b-icon icon="trash"></b-icon>
              </button>
            </b-table-column>
          </template>
        </b-table>
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
    
    beforeDestroy () {
      this.$options.sockets.onmessage = null;
    },
    
    async created () {
      this.$options.sockets.onmessage = this.handler;
      this.updateUsers();
    },

    computed: {
      paginationNeeded () {
        if ( this.users.length <= 10 ) {
          return false;
        } else {
          return true;
        }
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
        // We only care about publishes pertaining to a user resource.
        if ( msg.resource.type != 'user' ) {
          return;
        }

        let users = this.users;

        switch ( msg.resource.action ) {
          case 'create': {
            let user = msg.result;

            user.resource_names = user.resource_names.join( ' ' );
            users.push( user );
      
            this.users = [ ...users ];
          
            this.$buefy.toast.open({
              message: 'The ' + msg.resource.name + ' user was created.',
              type: 'is-success'
            });

            break;
          }

          case 'update': {
            for ( let i = 0; i < users.length; i++ ) {
              if ( users[i].username == msg.resource.name ) {
                let user = msg.result;

                user.resource_names = user.resource_names.join( ' ' );
                users[i] = user;

                break;
              }
            }
          
            this.users = [ ...users ];
          
            this.$buefy.toast.open({
              message: 'The ' + msg.resource.name + ' user was updated.',
              type: 'is-success'
            });

            break;
          }

          case 'delete': {
            for ( let i = 0; i < users.length; i++ ) {
              if ( users[i].username == msg.resource.name ) {
                users.splice( i, 1 );
                break;
              }
            }
          
            this.users = [ ...users ];
          
            this.$buefy.toast.open({
              message: 'The ' + msg.resource.name + ' user was deleted.',
              type: 'is-success'
            });

            break;
          }
        }
      },

      updateUsers () {
        this.$http.get( 'users' ).then(
          response => {
            response.json().then(
              state => {
                this.users = state.users;
                this.isWaiting = false;
              }
            );
          }, response => {
            this.$buefy.toast.open({
              message: 'Getting the users failed.',
              type: 'is-danger',
              duration: 4000
            });
            
            this.isWaiting = false;
          }
        );
      },
      
      adminUser () {
        return [ 'Global Admin', 'Experiment Admin' ].includes( this.$store.getters.role );
      },
      
      experimentUser () {
        return [ 'Global Admin', 'Experiment Admin', 'Experiment User' ].includes( this.$store.getters.role );
      },

      createUser () {
        for ( let i = 0; i < this.users.length; i++ ) {
          if ( this.users[i].username == this.user.username ) {
            this.userExists = true;
            return;
          }
        }

        if ( !this.user.username ) {
          this.$buefy.toast.open({
            message: 'You must include an username',
            type: 'is-warning',
            duration: 4000
          });

          return;
        }

        if ( !this.user.first_name ) {
          this.$buefy.toast.open({
            message: 'You must include a first name',
            type: 'is-warning',
            duration: 4000
          });

          return;
        }

        if ( !this.user.last_name ) {
          this.$buefy.toast.open({
            message: 'You must include a last name',
            type: 'is-warning',
            duration: 4000
          });

          return;
        }

        if ( !this.user.password ) {
          this.$buefy.toast.open({
            message: 'You must include a password',
            type: 'is-warning',
            duration: 4000
          });

          return;
        }

        if ( !this.user.confirmPassword ) {
          this.$buefy.toast.open({
            message: 'You must include a password confirmation',
            type: 'is-warning',
            duration: 4000
          });

          return;
        }

        if ( this.user.password != this.user.confirmPassword ) {
          this.$buefy.toast.open({
            message: 'Your passwords do not match',
            type: 'is-warning',
            duration: 4000
          });

          return;
        }

        delete this.user.confirmPassword;

        if ( !this.user.role_name ) {
          this.$buefy.toast.open({
            message: 'You must select a role',
            type: 'is-warning',
            duration: 4000
          });

          return;
        }

        if ( this.user.resource_names ) {
          this.user.resource_names = this.user.resource_names.split(' ');
        }

        this.isWaiting = true;
        
        let name = this.user.username;
        
        this.$http.post(
          'users', this.user
        ).then(
          response => {            
            this.isWaiting = false;
          }, response => {
            this.$buefy.toast.open({
              message: 'Creating the user ' + name + ' failed with ' + response.status + ' status.',
              type: 'is-danger',
              duration: 4000
            });
            
            this.isWaiting = false;
          }
        )

        this.resetLocalUser();
        this.isCreateActive = false;
      },

      editUser ( username ) {
        for ( let i = 0; i < this.users.length; i++ ) {
          if ( this.users[i].username == username ) {
            this.user = this.users[i];
            break;
          }
        }

        this.user.resource_names = _.uniq(this.user.resource_names).join(' ');

        this.isEditActive = true;
      },

      updateUser () {
        if ( this.$store.getters.username == this.user.username && this.$store.getters.role != this.user.role_name ) {
          this.$buefy.toast.open({
            message: 'You cannot change the role of the user you are currently logged in as.',
            type: 'is-danger',
            duration: 5000
          });
          
          this.resetLocalUser();
          this.isWaiting = false;
          this.isEditActive = false;
          
          return;
        }
        
        delete this.user.id;
        
        let user = this.user;

        user.resource_names = user.resource_names.split(' ');
        
        this.isEditActive = false;
        this.isWaiting = true;

        this.$http.patch( 
          'users/' + user.username, user 
        ).then(
          response => {
            let users = this.users;
                  
            for ( let i = 0; i < users.length; i++ ) {
              if ( users[i].username == user.username ) {
                users[i] = user;
                break;
              }
            }
            
            this.users = [ ...users ];       
            this.isWaiting = false;
          }, response => {
            this.$buefy.toast.open({
              message: 'Updating the ' + user.username + ' user failed with ' + response.status + ' status.',
              type: 'is-danger',
              duration: 4000
            });
            
            this.isWaiting = false;
          }
        )

        this.resetLocalUser();
      },

      deleteUser ( username ) {
        if ( username === this.$store.getters.username ) {
          this.$buefy.toast.open({
            message: 'You cannot delete the user you are currently logged in as.',
            type: 'is-danger',
            duration: 4000
          });

          return;
        }

        this.$buefy.dialog.confirm({
          title: 'Delete the User',
          message: 'This will DELETE the ' + username + ' user. Are you sure you want to do this?',
          cancelText: 'Cancel',
          confirmText: 'Delete',
          type: 'is-danger',
          hasIcon: true,
          onConfirm: () => {
            this.isWaiting = true;
            
            this.$http.delete(
              'users/' + username
            ).then(
              response => {
                let users = this.users;
                  
                for ( let i = 0; i < users.length; i++ ) {
                  if ( users[i].username == username ) {
                    users.splice( i, 1 );
                    break;
                  }
                }
                
                this.users = [ ...users ];
            
                this.isWaiting = false;
              }, response => {
                this.$buefy.toast.open({
                  message: 'Deleting the user ' + username + ' failed with ' + response.status + ' status.',
                  type: 'is-danger',
                  duration: 4000
                });
              }
            )
          }
        })
      },

      resetLocalUser () {
        this.user = {};
      }
    },

    data () {
      return {
        table: {
          striped: true,
          isPaginated: true,
          isPaginationSimple: true,
          paginationSize: 'is-small',
          defaultSortDirection: 'asc',
          currentPage: 1,
          perPage: 10
        },
        roles: [
          'Global Admin',
          'Experiment Admin',
          'Experiment User',
          'Experiment Viewer',
          'VM Viewer'
        ],
        users: [],
        user: {},
        userExists: false,
        isCreateActive: false,
        isEditActive: false,
        isWaiting: true
      }
    }
  }
</script>
