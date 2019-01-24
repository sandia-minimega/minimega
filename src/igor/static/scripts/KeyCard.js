(function() {
  const template = `
     <div class="card" style="margin-bottom:10px;">
       <div class="card-body" style="padding:0px;">
       <table class="table table-borderless">
         <tbody>
           <tr>
             <td></td>
             <td class="mdl key available clickable tdhover headtext text-center">
               Available
             </td>
             <td class="mdl key reserved clickable tdhover headtext text-center">
               Reserved
             </td>
           </tr>
           <tr>
             <td class="mdl key up clickable tdhover headtext text-right">
               Up
             </td>
             <td class="mdl key available up clickable tdhover">
               <div class="mdl mx-auto keycolor available up unselected">
               </div>
             </td>
             <td class="mdl key reserved up clickable tdhover">
               <div class="mdl mx-auto keycolor reserved up unselected">
               </div>
             </td>
           </tr>
           <tr>
             <td class="mdl key down clickable tdhover headtext text-right">
               Down
             </td>
             <td class="mdl key available down clickable tdhover">
               <div class="mdl mx-auto keycolor available down unselected">
               </div>
             </td>
             <td class="mdl key reserved down clickable tdhover">
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
  };
})();
