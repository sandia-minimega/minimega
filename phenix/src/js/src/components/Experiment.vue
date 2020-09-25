<!-- 
This component will display an experiment available to the user; 
it will include a specific component rendering based on whether 
the experiment is running or not. If the user is VM Viewer role, 
this will only show a list of VMs that a user can view.
 -->

<template>
  <component :is="component"></component>
</template>

<script>
  import Vue from 'vue'

  export default {
    //  These are the components available to the main experiment 
    //  component based on whether or not an experiment is running; 
    //  or, if the user is a VM Viewer role.
    components: {
      running: () => import('./RunningExperiment.vue'),
      stopped: () => import('./StoppedExperiment.vue'),
      vmtiles: () => import('./VMtiles.vue')
    },

    async beforeRouteEnter (to, _, next) {
      try {
        let resp = await Vue.http.get( 'experiments/' + to.params.id );
        let state = await resp.json();

        next(vm => vm.running = state.running);
      } catch (err) {
        console.log(err);

        Vue.toast.open({
          message: 'Getting the ' + to.params.id + ' experiment failed.',
          type: 'is-danger',
          duration: 4000
        });

        next();
      }
    },

    //  This computed value is based on the routing parameter 
    //  determined by the user clicking into an experiment from the 
    //  experiments component; or, based on the role type. The result 
    //  of the for loop is either to pass the VM Viewer component, or 
    //  a running or stopped experiment component.
    computed: {
      component: function () {
        if (this.running == null) {
          return
        }

        if (this.running == true) {
          if ( this.$store.getters.role === "VM Viewer" ) {
            return 'vmtiles';
          }

          return 'running';
        }

        return 'stopped';
      }
    },
    
    data () {
      return {
        running: null
      }
    }
  }
</script>
