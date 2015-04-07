function updateCol(table, v) {
	// Get the column API object
	var column = table.column($(v).attr('data-column'));
	var visible = $(v).prop('checked');

	if(visible) {
		// Hide everything that has a "" for the value for this column
		table.column(column).search(".+", true).draw();
	} else {
		// Unhide rows...
		table.column(column).search(".*", true).draw();
	}

	// Toggle the visibility
	column.visible(visible);

	table.draw();
}

$(document).ready(function() {
	var height = $(window).height();
	var width = $("#right").width();
	var table = $('#tags').DataTable({
		"scrollY": height-150,
		"paging": true,
		"stateSave": true,
		"stateSave": false,
		"scrollX": true,
		"lengthMenu": [[25, 50, 100, -1], [25, 50, 100, "All"]],
		"language": {
			"zeroRecords": "No VMs"
		},
	});

	$('#left input').each(function(i, v) {
		if (location.search.search($(v).attr('name')) > 0) {
			$(v).prop('checked', true);
		}

		updateCol(table, v);
	});

	$('#tags tbody').on('click', 'tr', function () {
		$(this).toggleClass('selected');
	});

	$('#left input').on('click', function (e) {
		updateCol(table, this);
	});
});
