<?php
//error_reporting(E_ALL);
//var_dump($_SERVER);
ini_set('log_errors', 1);
ini_set('display_errors', 0);
// save
if ($_SERVER['REQUEST_METHOD'] === 'POST') {
    header("Content-Type: application/json");
    $data = json_decode(file_get_contents("php://input"));
    if ($data == null) {
    	echo "Sorry, file could not be parsed as JSON.";
        return false;
    }
    $dir = '/phenix/topologies/'.basename($data->filename, ".json").'/';
    if (!file_exists($dir)) {
        mkdir($dir, 0777, true);
    }
    $file = $dir.$data->filename;
    if (file_exists($file)) {
        if ($data->overwrite) {
            $handle = fopen($file, "w");
            fwrite($handle, json_encode($data->data, JSON_PRETTY_PRINT));
            fclose($handle);
            echo "Topology saved successfully: ".$file;
        } else {
            echo "Sorry, topology already exists.";
            return false;
        }
    } else {
        $handle = fopen($file, "w");
        fwrite($handle, json_encode($data->data, JSON_PRETTY_PRINT));
        fclose($handle);
        echo "Topology saved successfully: ".$file;
    }
// import
} else if ($_SERVER['REQUEST_METHOD'] === 'GET') {
    $path = '/phenix/topologies/';
    if (isset($_GET['name'])) {
        $topo = $_GET['name'];
        $file = file_get_contents($path.$topo.'/'.$topo.'.json');
        echo $file;
    } else {
        if (!file_exists($path)) {
            echo "Sorry, no topologies were found.";
            return false;
        } else {
          $topos = array_filter(glob($path.'*/*.json'), 'is_file');
          // remove file extensions
          $topos = array_map(function($e) {
              return pathinfo($e, PATHINFO_FILENAME);
          }, $topos);
          echo json_encode($topos, JSON_UNESCAPED_SLASHES);
        }
    }
}
?>
