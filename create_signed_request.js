import crypto from "crypto";

function signRequest(secretKey, method, path, expires) {
  const data = `${method}\n${path}\n${expires}`;
  return crypto.createHmac('sha256', secretKey).update(data).digest('hex');
}

const accessKey = "3a0c876052f272deed948fe682e77dae";
const secretKey = "688fbfb6de25825153199abb7b9dbe41f1d7a6d949562351975ef590cd130024";
const method = "POST";
const path = "/api/presigned/url/upload?bucket=test&key=test1241";
const expires = Math.floor(Date.now() / 1000) + 3600; // valid for 1 hour

const signature = signRequest(secretKey, method, path, expires);
console.log(signature,expires);