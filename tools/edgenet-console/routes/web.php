<?php

use Illuminate\Support\Facades\Route;
use Illuminate\Support\Facades\Log;
use Illuminate\Http\Request;
/*
|--------------------------------------------------------------------------
| Web Routes
|--------------------------------------------------------------------------
|
| Here is where you can register web routes for your application. These
| routes are loaded by the RouteServiceProvider within a group which
| contains the "web" middleware group. Now create something great!
|
*/

Route::get('/password/reset/{token?}', function () {
    return view('console');
})->where('token', '.*');

Auth::routes(['register' => false]);
Route::post('/signup', 'K8s\SignupController@signup');

Route::get('/{any?}', function () {
    return view('console');
})->where('any', '.*');
