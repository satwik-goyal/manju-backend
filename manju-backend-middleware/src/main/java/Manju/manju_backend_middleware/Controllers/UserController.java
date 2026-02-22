package Manju.manju_backend_middleware.Controllers;

import Manju.manju_backend_middleware.Models.User;
import Manju.manju_backend_middleware.Services.UserService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import java.util.List;


@RestController
@RequestMapping("/api/users")
public class UserController {

    @Autowired
    private UserService userService;


    @PostMapping("/register")
    public User register(@RequestBody User user) {
        return userService.registerUser(user);
    }

    @PostMapping("/login")
    public String login(@RequestParam String username, @RequestParam String password) {
        return userService.login(username, password);
    }

    @GetMapping
    public List<User> getUsers() {
        return userService.getAllUsers();
    }
}
