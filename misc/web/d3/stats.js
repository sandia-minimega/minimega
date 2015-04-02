$(document).ready(function() {
	var height = $(window).height();
	var width = $(window).width();
	var table = $('#example').DataTable( {
		"scrollY": height-150,
		"paging": false,
		"stateSave": true,
		"scrollX": true,
		"columnDefs": [
		{
			"render": function ( data, type, row ) {
				var avg = data.split(" ")
				return avg[0] +' (1min), ' + avg[1] + ' (5min), '+ avg[2] + ' (15min)';
			},
			"targets": 2
		},
		{ "visible": true,  "targets": [ 2 ] }
		]
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
