<!-- 
This is the main file for the Vue app. It sets the header, footer,
and body to the overall Vue window. Header and Footer are separate
Vue components. There is a dispatch that is used to check for auto
login and returns a user to Experiments component if successful.
 -->

<template>
  <div class="container">
    <app-header></app-header>
    <div class="row">
      <div class="col-xs-12">
        <router-view></router-view>
      </div>
    <app-footer></app-footer>
    </div>
  </div>
</template>

<script>
  import Header from './components/Header.vue'
  import Footer from './components/Footer.vue'
  
  export default {
    components: {
      appHeader: Header,
      appFooter: Footer
    },
    
    beforeDestroy () {
      this.$disconnect();
      this.unwatch();
    },
    
    created () {
      const username = localStorage.getItem( 'user' );
      const token    = localStorage.getItem( 'token' );
      const role     = localStorage.getItem( 'role' );
      const auth     = localStorage.getItem( 'auth' );

      if ( token && auth ) {
        const user = {
          "username": username,
          "token":    token,
          "role":     role
        }

        this.$store.commit( 'LOGIN', { "user": user, "remember": true } );
      }

      this.wsConnect();

      this.unwatch = this.$store.watch(
        (_, getters) => getters.auth,
        () => {
          console.log(auth);

          // Disconnect the websocket client no matter what on auth changes.
          this.$disconnect();
          this.wsConnect();
        }
      )
    },

    methods: {
      wsConnect () {
        if (this.$store.getters.auth) {
          console.log('client authenticated -- initializing websocket');

          let path = '/api/v1/ws';

          if (this.$store.getters.token) {
            path += '?token=' + this.$store.getters.token;
          }

          this.$connect('//' + location.host + path);
        }
      }
    }
  }
</script>

<!-- 
This styling was based on some Buefy examples; there is some copied
values from the pervious phēnixweb styling. The rest was guessed at
until the window looked half way presentable. Otherwise, there is no
clue what this stuff does.
 -->

<style lang="scss">
  html {
    background-repeat: no-repeat;
    background-image: url("assets/phenix.png");
    background-size: background;
  }

  html, body {
    margin: 0;
    height: 100%;
  }

  body {
    padding: 20px;
    color: whitesmoke !important;
  }

  h1 {
    color: whitesmoke !important;
  }
  
  h3 {
    color: whitesmoke !important;
  }
  
  tr, td {
    color: whitesmoke !important;
  }
  
  th {
    background-color: #686868;
    color: whitesmoke !important;
  }
  
  a {
    color: whitesmoke !important;
  }
  
  p {
    color: #202020;
  }
  
  ul {
    columns: 2;
    -webkit-columns: 2;
    -moz-columns: 2;
  }
  
  li {
    color: whitesmoke !important;
  }

  #app {
    display: flex;
    flex-flow: column wrap;
    margin: 0 auto;
    height: 600px;
    justify-content: flex-start;
    align-content: flex-start;
  }

  #app > * {
    border-radius: 2px;
    transition: all ease 0.3s;
  }
  
  #app>div {
    position: relative;
    width: 200px;
    padding: 8px;
    margin: 10px;
    border: 1px solid #ccc;
  }
  
  textarea {
    display: block;
    width: 200px;
    height: 50px;
    padding: 8px;
    margin: 10px;
    border: 1px solid #ccc;
  }
  
  textarea:focus {
    border-color: black;
  }
  
  .top {
    text-align: right;
    display: flex;
    flex-direction: row-reverse;
    justify-content: space-between;
    margin-bottom: 0.5em;
  }

  .close {
    text-align: right;
    height: 10px;
    width: 10px;
    position: relative;
    box-sizing: border-box;
    line-height: 10px;
    display: inline-block;
  }
  
  .close:before, .close:after {
    transform: rotate(-45deg);
    content: "";
    position: absolute;
    top: 50%;
    left: 50%;
    margin-top: -1px;
    margin-left: -5px;
    display: block;
    height: 2px;
    width: 10px;
    background-color: black;
    transition: all 0.25s ease-out;
  }
  
  .close:after {
    transform: rotate(-135deg);
  }
  
  .close:hover:before, .close:hover:after {
    transform: rotate(0deg);
  }

  // Import Bulma's core
  @import "~bulma/sass/utilities/_all";

  $body-background-color: #333;
  $table-background-color: #484848;
  
  $button-text-color: whitesmoke;

  $light: #686868;
  $light-invert: findColorInvert($light);

  $progress-text-color: black;

  $colors: (
    "light": ($light, $light-invert),
    "dark": ($dark, $dark-invert),
    "white": ($white, $black),
    "black": ($black, $white),
    "primary": ($primary, $primary-invert),
    "info": ($info, $info-invert),
    "success": ($success, $success-invert),
    "warning": ($warning, $warning-invert),
    "danger": ($danger, $danger-invert)
  );
  
  // Import Bulma and Buefy styles
  @import "~bulma";
  @import "~buefy/src/scss/buefy";

  a.navbar-item:hover {
    background: #404040;
  }

  div.is-success {
    color: $success;
  }
</style>
