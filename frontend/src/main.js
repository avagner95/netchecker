import {Events} from "@wailsio/runtime";
import {App} from "../bindings/netchecker/internal/app";

let running = false;

document.getElementById('btnStartStop').addEventListener('click', () => {
    console.log("Starting app");
    if (!running)
        App.Start();
    else
        App.Stop()
})
console.log("Starting app");
console.log(App)
Events.On("app:size", (ev) => {
    console.log("Size:", ev.data);
    document.getElementById("db_size").innerText = "Size: "  + ev.data;

});

Events.On("app:running", (ev) => {
    running = ev.data;
    let btnText = running ? "Stop" : "Start";
    let StsText = running ? "Running" : "Stopped";
    let StsStatus = running ? "ok" : "muted";
    document.getElementById("btnStartStop").innerText = btnText;
    document.getElementById("app_sts_text").innerText = StsText;
    document.getElementById("app_sts_run").classList.remove('ok', 'muted')
    document.getElementById("app_sts_run").classList.add(StsStatus)


    // тут включай/выключай кнопки, показывай статус и т.д.
});
