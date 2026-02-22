package Manju.manju_backend_middleware.Services;

import Manju.manju_backend_middleware.Models.User;
import Manju.manju_backend_middleware.Repositories.UserRepository;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;


import java.util.List;
import java.util.Optional;


@Service
public class UserService {

    @Autowired
    private UserRepository userRepository;

    public User registerUser(User user){
        return userRepository.save(user);
    }

    public String login(String username, String password){
        Optional<User> user = userRepository.findByUsername(username);
        if (user.isPresent() && user.get().getPassword().equals(password)){
            return "Login Successful";
        }
        return "Invalid username or password";
    }

    public List<User> getAllUsers() {
        return userRepository.findAll();
    }
}
