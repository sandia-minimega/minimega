<!-- 
The header component is available on all views based on the 
App.vue component. The available routable links are available 
based on whether the user is logged in and furthermore based on 
their role. Based on the current limitations per user role, these 
are only available to Global Administrator or Global Viewer.
 -->

<template>
  <div>
    <a href="experiments">
      <img src="@/assets/phenix-banner.png" width="240">
    </a>
    <nav class="navbar is-light" role="navigation" aria-label="main navigation">
      <div id="navbarBasicExample" class="navbar-menu">
        <div class="navbar-start">
          <router-link v-if="auth && experimentUser()" 
                       to="/" 
                       class="navbar-item">Experiments</router-link>
          <router-link v-if="auth && experimentUser()" 
                       to="/hosts" 
                       class="navbar-item">Hosts</router-link>
          <router-link v-if="auth && globalAdmin()" 
                       to="/users" 
                       class="navbar-item">Users</router-link>
          <router-link v-if="auth && globalAdmin()" 
                       to="/log" 
                       class="navbar-item">Log</router-link>
        </div>
      </div>
      <div class="navbar-end">
        <div v-if="auth" class="navbar-item">
            <a role="button" class="button navbar-item is-light" @click="logout">Logout</a>
        </div>
      </div>

    </nav>
  </div>
</template>

<script>
  export default {
    //  The computed elements determine if the user is already logged 
    //  in; if so, the routable links are available. If not, the sign 
    //  in routable link is the only one available. The role getter 
    //  determines what the role of the user is; this is used to present 
    //  routable links in the header row.
    computed: {
      auth () {
        return this.$store.getters.auth;
      }
    },
    
    methods: {
      //  These methods are used to logout a user; or, present 
      //  routable link based on a Global user role.
      logout () {
        this.$http.get( 'logout' ).then(
          response => {
            if ( response.status == 204 ) {
              this.$store.commit( 'LOGOUT' );
            }
          }
        );
      },
      
      globalAdmin () {
        return [ 'Global Admin' ].includes( this.$store.getters.role );
      },
      
      adminUser () {
        return [ 'Global Admin', 'Experiment Admin' ].includes( this.$store.getters.role );
      },
      
      experimentUser () {
        return [ 'Global Admin', 'Experiment Admin', 'Experiment User', 'Experiment Viewer' ].includes( this.$store.getters.role );
      }
    }
  }
</script>
