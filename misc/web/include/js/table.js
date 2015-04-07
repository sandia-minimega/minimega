$(document).ready(function() {
	var height = $(window).height();
	var width = $(window).width();
	var table = $('#example').DataTable( {
		"scrollY": height-150,
		"paging": true,
		"stateSave": true,
		"scrollX": true,
		"lengthMenu": [[25, 50, 100, -1], [25, 50, 100, "All"]],
		"language": {
			"zeroRecords": "No VMs"
		}
	} );
	$('#example tbody').on( 'click', 'tr', function () {
		$(this).toggleClass('selected');
	} );

	$('a.toggle-vis').on( 'click', function (e) {
		e.preventDefault();

		// Get the column API object
		var column = table.column( $(this).attr('data-column') );

		// Toggle the visibility
		column.visible( ! column.visible() );
	} );
} );
