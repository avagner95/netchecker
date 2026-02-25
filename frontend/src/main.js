import {Events} from "@wailsio/runtime";
import {GreetService} from "../bindings/netchecker";

const resultElement = document.getElementById('result');
const timeElement = document.getElementById('time');

window.doGreet = async () => {
    let name = document.getElementById('name').value;
    if (!name) {
        name = 'anonymous';
    }
    try {
        resultElement.innerText = await GreetService.Greet(name);
    } catch (err) {
        console.error(err);
    }
}


Events.On("app:size", (ev) => {
    document.getElementById("db_size").innerText = "Size: "  + ev.data;
});
