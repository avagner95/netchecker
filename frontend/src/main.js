import {Events} from "@wailsio/runtime";
import {App} from "../bindings/netchecker/internal/app";

let running = false;

function setActiveStatus(status) {
    let btnText = status ? "Stop" : "Start";
    let StsText = status ? "Running" : "Stopped";
    let StsStatus = status ? "ok" : "muted";
    document.getElementById("btnStartStop").innerText = btnText;
    document.getElementById("app_sts_text").innerText = StsText;
    document.getElementById("app_sts_run").classList.remove('ok', 'muted')
    document.getElementById("app_sts_run").classList.add(StsStatus)
}

document.getElementById('btnStartStop').addEventListener('click', async () => {
    console.log("Starting app");
    if (!running)
        await App.Start();
    else
        await App.Stop()
})
console.log("Starting app");
console.log(App)
Events.On("app:size", (ev) => {
    console.log("Size:", ev.data);
    document.getElementById("db_size").innerText = "Size: "  + ev.data;

});

Events.On("app:running", (ev) => {
    running = ev.data;
    setActiveStatus(running);
    // тут включай/выключай кнопки, показывай статус и т.д.
});

window.addEventListener("DOMContentLoaded", async (event) => {
    console.log("Page is fully loaded");
    running = await App.IsRunning();
    console.log("App is running:", running);
    setActiveStatus(running);
});

