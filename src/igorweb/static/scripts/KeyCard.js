(function() {
  const template = `
     <div class="card" style="margin-bottom:10px;">
       <div class="card-body" style="padding:0px;">
       <table class="table table-borderless">
         <tbody>
           <tr>
             <td></td>
             <td class="mdl key available clickable tdhover headtext text-center" v-on:click.stop="select('available', null);">
               Available
             </td>
             <td class="mdl key reserved clickable tdhover headtext text-center" v-on:click.stop="select('reserved', null);">
               Reserved
             </td>
           </tr>
           <tr>
             <td class="mdl key up clickable tdhover headtext text-right" v-on:click.stop="select(null, 'up');">
               Up
             </td>
             <td class="mdl key available up clickable tdhover" v-on:click.stop="select('available', 'up');">
               <div class="mdl mx-auto keycolor available up unselected">
               </div>
             </td>
             <td class="mdl key reserved up clickable tdhover" v-on:click.stop="select('reserved', 'up')">
               <div class="mdl mx-auto keycolor reserved up unselected">
               </div>
             </td>
           </tr>
           <tr>
             <td class="mdl key down clickable tdhover headtext text-right" v-on:click.stop="select(null, 'down');">
               Down
             </td>
             <td class="mdl key available down clickable tdhover"  v-on:click.stop="select('available', 'down')">
               <div class="mdl mx-auto keycolor available down unselected">
               </div>
             </td>
             <td class="mdl key reserved down clickable tdhover" v-on:click.stop="select('reserved', 'down')">
               <div class="mdl mx-auto keycolor reserved down unselected">
               </div>
             </td>
           </tr>
         </tbody>
       </table>
     </div>
     </div>
    `;

  window.KeyCard = {
    template: template,

    methods: {
      select(availability, power) {
        let nodes = Object.values(this.$store.getters.nodes);

        let selected = nodes;
        if (availability == "available") {
          selected = nodes.filter((x) => x.Reservation == null);
        }
        if (availability == "reserved") {
          selected = nodes.filter((x) => x.Reservation != null);
        }

        if (power == "up") {
          selected = selected.filter((x) => x.Up);
        }
        if (power == "down") {
          selected = selected.filter((x) => !x.Up);
        }

        selected = selected.map((x) => x.NodeID);

        this.$store.dispatch("selectNodes", selected);
      },
    },
  };
})();
